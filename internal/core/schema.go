package core

import (
	"errors"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type ArtifactValidationError struct{ Message string }

func (e ArtifactValidationError) Error() string { return e.Message }

var artifactSchemaMap = map[string]string{
	"00-spec.yaml": "spec.schema.json", "01-blueprint.yaml": "blueprint.schema.json",
	"02-contract.yaml": "contract.schema.json", "04-implementation-log.yaml": "implementation-log.schema.json",
	"05-validation.json": "validation.schema.json", "06-review.json": "review.schema.json",
	"07-trace.json": "trace.schema.json", "feedback.json": "feedback.schema.json",
	"report.yaml": "report.schema.json",
}

func ValidateAgainstSchema(schemaName string, path string, payload any) error {
	schema, err := compileSchema(schemaName)
	if err != nil {
		return err
	}
	if err := schema.Validate(payload); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return err
		}
		chosen := chooseError(ve)
		return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=%s; reason=%s", path, jsonPointer(chosen.InstanceLocation), pythonReason(chosen, payload))}
	}
	if valFn, ok := postSchemaValidators[schemaName]; ok {
		if err := valFn(path, payload); err != nil {
			return err
		}
	}
	return nil
}

var postSchemaValidators = map[string]func(path string, payload any) error{
	"report.schema.json": validateReportSemanticInvariants,
}

func validateReportSemanticInvariants(path string, payload any) error {
	content, ok := lookupKey(payload, "content")
	if !ok || content == nil {
		return nil
	}

	sectionsVal, ok := lookupKey(content, "sections")
	if !ok || sectionsVal == nil {
		return nil
	}
	sections, ok := asSlice(sectionsVal)
	if !ok {
		return nil
	}

	// 1. Unicidad de section ID
	seenSectionIDs := make(map[string]int)
	for i, sec := range sections {
		idVal, ok := lookupKey(sec, "id")
		if !ok || idVal == nil {
			continue
		}
		idStr, ok := idVal.(string)
		if !ok {
			continue
		}
		if _, exists := seenSectionIDs[idStr]; exists {
			return ArtifactValidationError{
				Message: fmt.Sprintf("artifact=%s; field=$.content.sections[%d].id; reason=duplicate section id %q", path, i, idStr),
			}
		}
		seenSectionIDs[idStr] = i
	}

	// 2. Unicidad de findings.items[].id en todo el reporte
	type findingLocation struct {
		secIndex  int
		itemIndex int
	}
	seenFindingIDs := make(map[string]findingLocation)
	for i, sec := range sections {
		roleVal, _ := lookupKey(sec, "role")
		if roleVal == "findings" {
			itemsVal, ok := lookupKey(sec, "items")
			if ok && itemsVal != nil {
				items, _ := asSlice(itemsVal)
				for j, item := range items {
					idVal, _ := lookupKey(item, "id")
					if idVal == nil {
						continue
					}
					idStr, ok := idVal.(string)
					if !ok {
						continue
					}
					if _, exists := seenFindingIDs[idStr]; exists {
						return ArtifactValidationError{
							Message: fmt.Sprintf("artifact=%s; field=$.content.sections[%d].items[%d].id; reason=duplicate finding id %q", path, i, j, idStr),
						}
					}
					seenFindingIDs[idStr] = findingLocation{secIndex: i, itemIndex: j}
				}
			}
		}
	}

	// 3. Integridad de evidence.items[].findingId -> findings.items[].id
	for i, sec := range sections {
		roleVal, _ := lookupKey(sec, "role")
		if roleVal == "evidence" {
			itemsVal, ok := lookupKey(sec, "items")
			if ok && itemsVal != nil {
				items, _ := asSlice(itemsVal)
				for j, item := range items {
					findingIdVal, _ := lookupKey(item, "findingId")
					if findingIdVal == nil {
						continue
					}
					findingIdStr, ok := findingIdVal.(string)
					if !ok {
						continue
					}
					if _, exists := seenFindingIDs[findingIdStr]; !exists {
						return ArtifactValidationError{
							Message: fmt.Sprintf("artifact=%s; field=$.content.sections[%d].items[%d].findingId; reason=unknown finding id %q", path, i, j, findingIdStr),
						}
					}
				}
			}
		}
	}

	return nil
}

func lookupKey(obj any, key string) (any, bool) {
	if obj == nil {
		return nil, false
	}
	if m, ok := obj.(map[string]any); ok {
		val, found := m[key]
		return val, found
	}
	if m, ok := obj.(map[any]any); ok {
		val, found := m[key]
		return val, found
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Map {
		for _, k := range rv.MapKeys() {
			if k.Kind() == reflect.String && k.String() == key {
				return rv.MapIndex(k).Interface(), true
			}
			if fmt.Sprintf("%v", k.Interface()) == key {
				return rv.MapIndex(k).Interface(), true
			}
		}
	}
	return nil, false
}


func ValidateArtifact(path string, payload any) error {
	name, ok := artifactSchemaName(path)
	if !ok {
		return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$; reason=unsupported artifact path", path)}
	}
	return ValidateAgainstSchema(name, path, payload)
}

func artifactSchemaName(path string) (string, bool) {
	if schema, ok := artifactSchemaMap[filepath.Base(path)]; ok {
		return schema, true
	}
	if filepath.Ext(path) == ".json" {
		for _, p := range strings.Split(filepath.ToSlash(path), "/") {
			if p == "memory" {
				return "memory.schema.json", true
			}
		}
	}
	if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
		for _, p := range strings.Split(filepath.ToSlash(path), "/") {
			if p == "reports" {
				return "report.schema.json", true
			}
		}
	}
	return "", false
}

func compileSchema(name string) (*jsonschema.Schema, error) {
	dir, err := SchemasDir()
	if err != nil {
		return nil, err
	}
	loc := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, name))}).String()
	return jsonschema.NewCompiler().Compile(loc)
}

func SchemasDir() (string, error) {
	if env := os.Getenv("QUORUM_SCHEMAS_DIR"); env != "" {
		return env, nil
	}
	starts := []string{}
	if root, err := ProjectRoot(); err == nil {
		starts = append(starts, root)
	}
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	seen := map[string]bool{}
	for _, start := range starts {
		for dir := filepath.Clean(start); !seen[dir]; dir = filepath.Dir(dir) {
			seen[dir] = true
			p := filepath.Join(dir, ".agents", "schemas")
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p, nil
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", fmt.Errorf("schemas directory not found")
}

func chooseError(root *jsonschema.ValidationError) *jsonschema.ValidationError {
	leaves := validationLeaves(root)
	for _, e := range leaves {
		if _, ok := e.ErrorKind.(*kind.Required); ok {
			return e
		}
	}
	best := leaves[0]
	for _, e := range leaves[1:] {
		if jsonPointer(e.InstanceLocation) >= jsonPointer(best.InstanceLocation) {
			best = e
		}
	}
	return best
}

func validationLeaves(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	out := []*jsonschema.ValidationError{}
	for _, c := range e.Causes {
		out = append(out, validationLeaves(c)...)
	}
	return out
}

func pythonReason(e *jsonschema.ValidationError, payload any) string {
	value := valueAt(payload, e.InstanceLocation)
	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		return fmt.Sprintf("%s is a required property", pyRepr(k.Missing[0]))
	case *kind.AdditionalProperties:
		props := append([]string(nil), k.Properties...)
		sort.Strings(props)
		q := make([]string, len(props))
		for i, p := range props {
			q[i] = pyRepr(p)
		}
		if len(q) == 1 {
			return fmt.Sprintf("Additional properties are not allowed (%s was unexpected)", q[0])
		}
		return fmt.Sprintf("Additional properties are not allowed (%s were unexpected)", strings.Join(q, ", "))
	case *kind.MinItems:
		if k.Want == 1 && k.Got == 0 {
			return "[] should be non-empty"
		}
		return fmt.Sprintf("%s is too short", pyRepr(value))
	case *kind.MaxItems:
		return fmt.Sprintf("%s is too long", pyRepr(value))
	case *kind.MinLength:
		if k.Want == 1 && k.Got == 0 {
			return "'' should be non-empty"
		}
		return fmt.Sprintf("%s is too short", pyRepr(value))
	case *kind.MaxLength:
		return fmt.Sprintf("%s is too long", pyRepr(value))
	case *kind.Pattern:
		return fmt.Sprintf("%s does not match %s", pyRepr(k.Got), pyRepr(k.Want))
	case *kind.Minimum:
		return fmt.Sprintf("%s is less than the minimum of %s", pyRepr(value), ratRepr(k.Want))
	case *kind.Maximum:
		return fmt.Sprintf("%s is greater than the maximum of %s", pyRepr(value), ratRepr(k.Want))
	case *kind.Enum:
		return fmt.Sprintf("%s is not one of %s", pyRepr(k.Got), pyRepr(k.Want))
	case *kind.Const:
		return fmt.Sprintf("%s was expected", pyRepr(k.Want))
	case *kind.Type:
		return fmt.Sprintf("%s is not of type %s", pyRepr(value), quotedJoin(k.Want))
	default:
		// LocalizedString panics on a nil printer (e.g. kind.MaxItems), so hand it
		// a real one as the safe fallback for any kind not rendered explicitly above.
		return e.ErrorKind.LocalizedString(message.NewPrinter(language.English))
	}
}

func jsonPointer(parts []string) string {
	out := "$"
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err == nil {
			out += "[" + p + "]"
		} else {
			out += "." + p
		}
	}
	return out
}

func valueAt(v any, path []string) any {
	for _, p := range path {
		if m, ok := v.(map[string]any); ok {
			v = m[p]
			continue
		}
		idx, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		if s, ok := asSlice(v); ok && idx >= 0 && idx < len(s) {
			v = s[idx]
			continue
		}
		return nil
	}
	return v
}

func asSlice(v any) ([]any, bool) {
	if s, ok := v.([]any); ok {
		return s, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}
func ratRepr(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	f, _ := r.Float64()
	return strconv.FormatFloat(f, 'f', -1, 64)
}
func quotedJoin(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = pyRepr(x)
	}
	return strings.Join(q, ", ")
}
func pyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(x, "\\", "\\\\"), "'", "\\'") + "'"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case []any:
		p := []string{}
		for _, item := range x {
			p = append(p, pyRepr(item))
		}
		return "[" + strings.Join(p, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}
