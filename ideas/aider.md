# Política de uso de Aider para ahorro de tokens en tareas de desarrollo

## 1. Objetivo

Este documento define en qué situaciones debe usarse Aider como herramienta auxiliar para reducir consumo de tokens y costo operativo en tareas de desarrollo de software.

La premisa principal es:

> Aider debe usarse cuando la tarea puede resolverse con edición localizada, instrucciones claras, archivos acotados y validación automática mediante tests, lint o typecheck.

Aider no debe reemplazar al agente principal en tareas de arquitectura, análisis ambiguo o decisiones de producto. Su rol recomendado es ser un ejecutor barato de cambios concretos.

---

## 2. Principio rector

Aider ahorra tokens cuando se evita que un agente autónomo explore todo el repositorio, abra muchos archivos, razone de forma amplia o mantenga una conversación larga.

El ahorro ocurre principalmente por cinco motivos:

1. Se le entregan archivos específicos en vez de todo el proyecto.
2. Usa un mapa del repositorio para orientarse sin leer todo el contenido.
3. Puede trabajar con modelos más económicos para tareas mecánicas.
4. Edita mediante diffs o cambios localizados, reduciendo tokens de salida.
5. Puede usar salidas concretas de test, lint o compilación para corregir errores sin reanalizar todo el sistema.

Por eso, Aider debe usarse solo cuando el problema ya tiene un perímetro claro.

---

## 3. Situaciones donde Aider debería usarse

Las siguientes situaciones son recomendadas porque, bien ejecutadas, deberían ahorrar tokens frente a usar un agente autónomo pesado para todo el ciclo.

---

## 3.1 Refactor mecánico en archivos conocidos

### Cuándo usar Aider

Usar Aider cuando el cambio sea repetitivo, localizado y tenga una regla clara.

Ejemplos:

* Renombrar una variable, función, prop, hook, action o selector.
* Cambiar una importación repetida.
* Actualizar una firma de función en varios archivos conocidos.
* Cambiar nombres de campos en formularios, reducers, serializers o DTOs.
* Reemplazar una API interna antigua por una nueva cuando el patrón es evidente.

### Por qué ahorra tokens

Un agente autónomo normalmente podría explorar carpetas, buscar referencias, abrir múltiples archivos y razonar de más sobre la arquitectura.

Con Aider, el agente principal puede pasarle únicamente los archivos afectados y una instrucción concreta. Eso reduce contexto de entrada y evita ciclos largos de exploración.

### Cómo usarlo

Primero identificar archivos con una herramienta determinística:

```bash
rg "oldName" src/
```

Luego ejecutar Aider solo sobre esos archivos:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Renombra oldName a newName preservando comportamiento. No cambies lógica no relacionada." \
  src/file1.ts src/file2.ts src/file3.ts
```

### Criterio de aceptación

Aider debe tocar únicamente los archivos esperados y el diff debe ser mecánico, pequeño y fácil de revisar.

---

## 3.2 Corrección de errores de lint en archivos específicos

### Cuándo usar Aider

Usar Aider cuando el linter ya indica errores concretos.

Ejemplos:

* Imports no usados.
* Tipos faltantes.
* Variables no usadas.
* Reglas de ESLint, Ruff, Flake8, Oxlint o TypeScript.
* Errores de formato que el autofix no resolvió.

### Por qué ahorra tokens

El agente no necesita investigar el repositorio. La salida del linter ya dice dónde está el problema. Aider puede trabajar directamente sobre los archivos señalados.

### Cómo usarlo

Primero intentar autofix determinístico:

```bash
npm run lint -- --fix
```

Si quedan errores:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --lint-cmd "npm run lint" \
  --auto-lint \
  src/file-with-errors.ts
```

También puede usarse con una instrucción explícita:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Corrige únicamente los errores de lint en este archivo. No cambies comportamiento." \
  src/file-with-errors.ts
```

### Criterio de aceptación

El lint debe pasar y el cambio no debe alterar comportamiento funcional.

---

## 3.3 Corrección de errores de typecheck

### Cuándo usar Aider

Usar Aider cuando TypeScript, mypy, pyright u otra herramienta indique errores concretos en archivos específicos.

Ejemplos:

* Tipos incompatibles.
* Props mal definidas.
* Retornos incorrectos.
* Tipos faltantes.
* Cambios de API que rompieron consumidores conocidos.

### Por qué ahorra tokens

El typecheck entrega una ubicación precisa y un mensaje técnico. Aider no necesita recorrer la arquitectura completa, solo corregir el contrato en los archivos involucrados.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Corrige los errores de typecheck en estos archivos sin cambiar comportamiento ni agregar dependencias." \
  src/module/a.ts src/module/b.ts
```

Después validar:

```bash
npm run typecheck
```

### Criterio de aceptación

El typecheck debe pasar y no debe haber cambios fuera del módulo afectado.

---

## 3.4 Generación de pruebas unitarias para una función, hook, reducer o componente específico

### Cuándo usar Aider

Usar Aider cuando ya se sabe qué unidad se quiere probar.

Ejemplos:

* Tests para un reducer.
* Tests para un hook.
* Tests para una función utilitaria.
* Tests para un serializer.
* Tests para una validación de formulario.
* Tests para un bug ya reproducido.

### Por qué ahorra tokens

Escribir tests con un agente autónomo suele ser caro porque el agente explora estructura, configuración, helpers, fixtures y patrones existentes.

Con Aider, se le pasan solo el archivo productivo, el archivo de test y, si hace falta, un helper de referencia en modo lectura. Esto reduce drásticamente el contexto.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --test-cmd "npm test -- useCliente.test.ts" \
  --message "Agrega pruebas unitarias para los casos indicados. No cambies la implementación salvo que encuentres un bug evidente." \
  src/hooks/useCliente.ts src/hooks/useCliente.test.ts
```

### Criterio de aceptación

Los tests nuevos deben fallar si se rompe el comportamiento esperado y deben pasar con la implementación actual.

---

## 3.5 Reparación de pruebas fallidas

### Cuándo usar Aider

Usar Aider cuando existe una prueba fallida con salida clara.

Ejemplos:

* Test snapshot roto.
* Mock desactualizado.
* Cambio de firma que rompió tests.
* Assertion incorrecta.
* Setup incompleto.
* Error de importación en test.

### Por qué ahorra tokens

Aider puede usar la salida exacta del test fallido. No necesita leer todo el proyecto ni adivinar qué revisar.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --test-cmd "npm test -- clienteReducer.test.ts" \
  --auto-test \
  src/reducers/clienteReducer.js src/reducers/clienteReducer.test.js
```

O dentro de Aider:

```text
/test npm test -- clienteReducer.test.ts
```

### Criterio de aceptación

Debe corregir la causa específica del fallo, no simplemente cambiar el test para que pase sin validar comportamiento real.

---

## 3.6 Migraciones de sintaxis repetitiva

### Cuándo usar Aider

Usar Aider cuando una librería cambió una API y la migración sigue un patrón claro.

Ejemplos:

* Cambiar imports antiguos por nuevos.
* Actualizar llamadas de una función obsoleta.
* Migrar una opción de configuración.
* Reemplazar un componente wrapper por otro.
* Cambiar sintaxis de hooks, actions o middlewares.

### Por qué ahorra tokens

La tarea es principalmente transformación de código. El agente principal no necesita razonar profundamente en cada archivo. Aider puede aplicar la regla sobre una lista acotada de archivos.

### Cómo usarlo

Primero listar ocurrencias:

```bash
rg "deprecatedApi" src/
```

Luego ejecutar por lote pequeño:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Migra deprecatedApi a newApi siguiendo el patrón existente. Mantén comportamiento y no hagas refactors adicionales." \
  src/module/file1.ts src/module/file2.ts
```

### Criterio de aceptación

Cada lote debe ser pequeño. Si la migración empieza a requerir decisiones de arquitectura, detener Aider y volver al agente principal.

---

## 3.7 Agregar docstrings, JSDoc o type hints a archivos concretos

### Cuándo usar Aider

Usar Aider cuando se quiere documentar código específico, no todo el repositorio.

Ejemplos:

* Agregar JSDoc a funciones exportadas.
* Agregar docstrings a servicios Python.
* Agregar comentarios técnicos a validaciones complejas.
* Agregar type hints a funciones puntuales.
* Documentar contratos públicos internos.

### Por qué ahorra tokens

Un agente autónomo puede intentar entender toda la arquitectura antes de documentar. Aider puede operar sobre un archivo concreto, usando el contexto mínimo necesario.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Agrega JSDoc solo a las funciones exportadas. No cambies lógica ni nombres." \
  src/utils/tax.ts
```

### Criterio de aceptación

La documentación debe explicar intención, parámetros, retornos y casos límite sin introducir cambios funcionales.

---

## 3.8 Cambios de copy, labels, mensajes de error o textos de UI en archivos conocidos

### Cuándo usar Aider

Usar Aider cuando el cambio de texto está claramente definido.

Ejemplos:

* Cambiar mensajes de validación.
* Mejorar labels de botones.
* Ajustar textos de ayuda.
* Reemplazar terminología de dominio.
* Cambiar “NIT” por “RFC” en módulos específicos.
* Ajustar banners, tooltips o mensajes de error.

### Por qué ahorra tokens

El cambio suele ser textual y localizado. No justifica que un agente caro analice arquitectura, rutas o estado global.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Reemplaza el término NIT por RFC solo en textos visibles de UI para cuentas México. No cambies nombres internos de variables." \
  src/pages/clientes/FormCliente.tsx src/components/CustomerSearch.tsx
```

### Criterio de aceptación

Debe cambiar copy visible sin romper variables, claves de traducción ni contratos internos.

---

## 3.9 Correcciones pequeñas después de una revisión de PR

### Cuándo usar Aider

Usar Aider cuando el code review dejó comentarios concretos.

Ejemplos:

* “Extrae esta condición a una función.”
* “Agrega este caso de test.”
* “Renombra esta variable.”
* “Evita duplicación en estos dos bloques.”
* “Corrige este import.”
* “Agrega manejo de null.”

### Por qué ahorra tokens

Los comentarios de review ya son instrucciones de cambio. Aider puede aplicarlas sin reabrir toda la discusión técnica.

### Cómo usarlo

Copiar los comentarios relevantes en un prompt acotado:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message-file .aider-review-fixes.md \
  src/file1.ts src/file1.test.ts
```

### Criterio de aceptación

Cada comentario debe quedar resuelto con un diff pequeño y validable.

---

## 3.10 Aplicar un plan técnico ya diseñado por un agente fuerte

### Cuándo usar Aider

Usar Aider cuando Claude Code, Codex, OpenCode, Antigravity u otro agente ya diseñó el plan y solo falta ejecutar cambios de código.

Ejemplos:

* El agente fuerte definió qué archivos modificar.
* El agente fuerte definió criterios de aceptación.
* El agente fuerte descartó riesgos arquitectónicos.
* El cambio se puede partir en pasos mecánicos.

### Por qué ahorra tokens

Se evita que el modelo fuerte consuma tokens escribiendo todos los cambios. El modelo fuerte razona; Aider ejecuta.

### Cómo usarlo

El agente principal debe generar un archivo `.aider-task.md`:

```md
Objective:
Aplicar el plan técnico ya aprobado.

Allowed files:
- src/file1.ts
- src/file2.ts

Constraints:
- No agregar dependencias.
- No cambiar APIs públicas.
- Mantener diff mínimo.

Validation:
Run: npm test -- file1.test.ts
```

Luego:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message-file .aider-task.md \
  src/file1.ts src/file2.ts
```

### Criterio de aceptación

Aider no debe reinterpretar el plan. Solo debe ejecutarlo.

---

## 3.11 Limpieza de deuda pequeña y localizada

### Cuándo usar Aider

Usar Aider cuando la deuda técnica está claramente ubicada y no requiere rediseño.

Ejemplos:

* Eliminar código muerto en un archivo.
* Simplificar una condición repetida.
* Extraer una función pequeña.
* Reducir duplicación local.
* Ordenar imports.
* Separar una función larga en helpers internos.

### Por qué ahorra tokens

La tarea no necesita exploración global. Si el alcance está bien delimitado, Aider puede resolverlo con menos contexto que un agente autónomo.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Limpia deuda local en este archivo: elimina duplicación obvia y extrae helpers privados si mejora legibilidad. No cambies comportamiento." \
  src/module/service.ts
```

### Criterio de aceptación

El diff debe ser local, sin cambios de API ni efectos colaterales.

---

## 3.12 Adaptación de tests después de cambios mecánicos

### Cuándo usar Aider

Usar Aider cuando un refactor mecánico rompió tests por nombres, imports o mocks desactualizados.

Ejemplos:

* Se renombró una función y los tests importan el nombre anterior.
* Cambió una prop y los tests renderizan el componente con la prop antigua.
* Cambió un mock por una nueva firma.
* Cambió un selector y el test usa el selector viejo.

### Por qué ahorra tokens

Aider puede trabajar sobre el test fallido y el archivo productivo relacionado. No necesita revisar todo el sistema.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --test-cmd "npm test -- affected.test.ts" \
  --auto-test \
  src/module/affected.ts src/module/affected.test.ts
```

### Criterio de aceptación

El test debe seguir validando el comportamiento correcto, no simplemente adaptarse a una implementación incorrecta.

---

## 3.13 Cambios repetitivos en configuración no crítica

### Cuándo usar Aider

Usar Aider cuando hay archivos de configuración repetitivos, pero de bajo riesgo.

Ejemplos:

* Ajustar scripts de lint.
* Homologar aliases.
* Ordenar configuraciones.
* Cambiar paths de tooling.
* Actualizar nombres de comandos internos.

### Por qué ahorra tokens

La modificación suele ser textual y verificable. No requiere razonamiento profundo si el cambio está bien descrito.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Actualiza estos scripts para usar el nuevo comando lint:quick. No cambies dependencias ni versiones." \
  package.json .gitlab-ci.yml
```

### Criterio de aceptación

Debe validarse ejecutando el comando afectado o revisando que el pipeline mantenga su intención.

---

## 3.14 Resolución de conflictos simples de Git

### Cuándo usar Aider

Usar Aider solo cuando el conflicto sea pequeño, textual y fácil de validar.

Ejemplos:

* Ambos lados agregaron imports.
* Ambos lados editaron copy cercano.
* Ambos lados modificaron una lista simple.
* Conflicto en tests por nombres actualizados.
* Conflicto en archivos de configuración no críticos.

### Por qué ahorra tokens

Aider puede enfocarse en las marcas de conflicto y el contexto inmediato, evitando que un agente autónomo revise historial, ramas o arquitectura completa.

### Cómo usarlo

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Resuelve solo los conflictos de este archivo preservando intención de ambas ramas. No hagas refactors adicionales." \
  src/file-with-conflict.ts
```

Luego validar:

```bash
git diff
npm test -- related.test.ts
```

### Criterio de aceptación

El conflicto debe quedar resuelto, el archivo debe compilar y el diff debe ser fácil de revisar manualmente.

---

## 4. Cómo debe usarse Aider en general

## 4.1 Primero acotar el trabajo

Antes de ejecutar Aider, siempre debe definirse:

* objetivo;
* archivos permitidos;
* archivos prohibidos;
* restricciones;
* criterio de aceptación;
* comando de validación.

No debe invocarse Aider con instrucciones amplias como:

```text
Arregla este módulo.
```

Debe usarse con instrucciones acotadas como:

```text
Corrige los errores de typecheck en estos dos archivos sin cambiar APIs públicas.
```

---

## 4.2 Usar modelos baratos por defecto

Aider debe usar un modelo barato cuando la tarea sea mecánica.

Ejemplos de categorías adecuadas:

* modelos Flash;
* modelos DeepSeek;
* modelos Llama;
* modelos locales si son suficientemente buenos para edición;
* modelos económicos compatibles con el proveedor configurado.

El modelo fuerte debe reservarse para:

* diseño;
* diagnóstico complejo;
* revisión final;
* tareas donde el modelo barato falle dos veces.

---

## 4.3 Trabajar por lotes pequeños

No se debe pedir a Aider que modifique 40 archivos en un solo paso salvo que el cambio sea extremadamente trivial.

Regla recomendada:

* 1 a 5 archivos para cambios con lógica.
* 5 a 15 archivos para cambios puramente mecánicos.
* Más de 15 archivos solo si se validan por lotes y el patrón es muy claro.

---

## 4.4 Revisar siempre el diff

Después de Aider, siempre ejecutar:

```bash
git diff --stat
git diff
```

Se debe verificar:

* qué archivos tocó;
* si tocó archivos no autorizados;
* si agregó dependencias;
* si cambió APIs públicas;
* si el diff es más grande de lo esperado;
* si alteró lógica no relacionada.

---

## 4.5 Validar con comandos concretos

Toda ejecución de Aider debe terminar con una validación.

Ejemplos:

```bash
npm run lint
npm run typecheck
npm test -- affected.test.ts
pytest tests/test_specific.py
ruff check .
```

Si no existe validación automática, debe hacerse revisión manual del diff y marcar la tarea como de mayor riesgo.

---

## 4.6 Permitir máximo dos intentos

Aider debe tener un límite de reintentos.

Regla recomendada:

1. Primer intento: aplicar cambio.
2. Segundo intento: corregir fallo de lint/test/typecheck.
3. Si falla otra vez: detener y escalar al agente principal o a revisión humana.

Esto evita que el ahorro de tokens desaparezca por ciclos repetidos.

---

## 5. Cuándo no usar Aider

No usar Aider cuando la tarea requiera exploración, arquitectura o juicio amplio.

Evitar Aider en:

* diseño de arquitectura;
* bugs ambiguos sin reproducción;
* cambios de seguridad;
* migraciones de base de datos;
* cambios con impacto contable, fiscal o legal;
* decisiones de producto;
* refactors grandes sin plan previo;
* cambios que afecten muchos módulos desconocidos;
* conflictos de Git donde ambas ramas cambian lógica de negocio;
* upgrades de dependencias con breaking changes no analizados.

En esos casos, usar primero un agente fuerte para diagnóstico, diseño y plan. Aider puede entrar después, solo si el plan se transforma en tareas localizadas.

---

## 6. Plantilla recomendada para tareas de Aider

```md
Objective:
<describir un único objetivo claro>

Allowed files:
- <archivo 1>
- <archivo 2>

Do not edit:
- migrations
- lockfiles
- generated files
- unrelated modules
- secrets or environment files

Constraints:
- Preserve existing behavior unless explicitly stated.
- Keep the diff minimal.
- Do not add dependencies.
- Do not change public APIs unless explicitly requested.
- Do not perform unrelated refactors.

Acceptance criteria:
- <criterio 1>
- <criterio 2>
- <criterio 3>

Validation:
Run: <comando de test/lint/typecheck>
```

---

## 7. Comando base recomendado

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message-file .aider-task.md \
  --auto-lint \
  --test-cmd "<validation command>" \
  <allowed files>
```

Para tareas muy simples:

```bash
aider \
  --model "$AIDER_CHEAP_MODEL" \
  --message "Aplica este cambio mecánico manteniendo comportamiento y diff mínimo." \
  <allowed files>
```

---

## 8. Criterio final de decisión

Usar Aider cuando se cumplan estas condiciones:

1. La tarea tiene alcance claro.
2. Los archivos afectados se pueden listar antes de empezar.
3. El cambio es mecánico, localizado o ya diseñado.
4. Existe una forma de validar el resultado.
5. El riesgo de negocio es bajo o controlado.
6. El diff esperado es pequeño o repetitivo.
7. El modelo barato es suficiente para ejecutar, no para decidir.

Si estas condiciones se cumplen, Aider debería ahorrar tokens frente a delegar todo el trabajo a un agente autónomo pesado.

---

## 9. Resumen ejecutivo

Aider debe usarse como ejecutor económico para cambios concretos.

Debe usarse especialmente en:

* refactors mecánicos;
* lint fixes;
* typecheck fixes;
* tests unitarios;
* reparación de tests;
* docstrings;
* cambios de copy;
* PR review fixes;
* aplicación de planes ya diseñados;
* limpieza local;
* migraciones repetitivas;
* conflictos simples.

No debe usarse como arquitecto principal ni como investigador de bugs ambiguos.

La fórmula recomendada es:

> Agente fuerte para pensar. Aider barato para editar. Tests y diff para validar.
