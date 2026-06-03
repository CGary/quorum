document.addEventListener('DOMContentLoaded', () => {
    const projectSelect = document.getElementById('project-select');
    const reportList = document.getElementById('report-list');
    const reportTitle = document.getElementById('report-title');
    const reportDate = document.getElementById('report-date');
    const reportContent = document.getElementById('report-content');

    let currentProject = '';

    // Fetch projects on load
    fetch('/api/projects')
        .then(res => res.json())
        .then(data => {
            projectSelect.innerHTML = '<option value="" disabled selected>Select a project...</option>';
            if (data && data.length > 0) {
                data.forEach(p => {
                    const opt = document.createElement('option');
                    opt.value = p.id;
                    opt.textContent = p.name || p.id;
                    projectSelect.appendChild(opt);
                });
            } else {
                projectSelect.innerHTML = '<option value="" disabled selected>No projects found</option>';
            }
        })
        .catch(err => {
            console.error('Error fetching projects:', err);
            projectSelect.innerHTML = '<option value="" disabled selected>Error loading projects</option>';
        });

    projectSelect.addEventListener('change', (e) => {
        currentProject = e.target.value;
        loadReports(currentProject);
    });

    function loadReports(projectId) {
        reportList.innerHTML = '<div class="empty-state">Loading reports...</div>';
        fetch(`/api/projects/${projectId}/reports`)
            .then(res => res.json())
            .then(data => {
                reportList.innerHTML = '';
                if (data && data.length > 0) {
                    data.forEach(r => {
                        const item = document.createElement('div');
                        item.className = 'report-item';

                        const title = document.createElement('div');
                        title.className = 'report-item-title';
                        title.textContent = r.id;

                        const date = document.createElement('div');
                        date.className = 'report-item-date';
                        const d = new Date(r.updated_at);
                        date.textContent = isNaN(d.getTime()) ? r.updated_at : d.toLocaleString();

                        item.appendChild(title);
                        item.appendChild(date);

                        item.addEventListener('click', () => {
                            document.querySelectorAll('.report-item').forEach(el => el.classList.remove('active'));
                            item.classList.add('active');
                            loadReportDetail(projectId, r.id);
                        });

                        reportList.appendChild(item);
                    });
                } else {
                    reportList.innerHTML = '<div class="empty-state">No reports found</div>';
                }
            })
            .catch(err => {
                console.error('Error fetching reports:', err);
                reportList.innerHTML = '<div class="empty-state">Error loading reports</div>';
            });
    }

    function loadReportDetail(projectId, reportId) {
        reportContent.innerHTML = '<div class="empty-state">Loading report...</div>';
        reportTitle.textContent = `Loading ${reportId}...`;
        reportDate.textContent = '';

        fetch(`/api/projects/${projectId}/reports/${reportId}`)
            .then(res => {
                if (!res.ok) throw new Error('Failed to load report');
                return res.json();
            })
            .then(data => {
                renderReport(reportId, data);
            })
            .catch(err => {
                console.error('Error fetching report details:', err);
                reportTitle.textContent = 'Error';
                reportContent.innerHTML = '<div class="empty-state">Failed to load report details.</div>';
            });
    }

    function renderReport(reportId, data) {
        reportTitle.textContent = data.meta?.id || reportId;
        if (data.meta?.date) {
            const d = new Date(data.meta.date);
            reportDate.textContent = isNaN(d.getTime()) ? data.meta.date : d.toLocaleString();
        } else {
            reportDate.textContent = '';
        }

        reportContent.innerHTML = ''; // Safe because we just set it to empty string

        // Safe helper to create elements using textContent.
        const el = (tag, className, text) => {
            const element = document.createElement(tag);
            if (className) element.className = className;
            if (text !== undefined) element.textContent = text;
            return element;
        };

        const titleize = (key) => key
            .replace(/([A-Z])/g, ' $1')
            .replace(/^./, c => c.toUpperCase())
            .trim();

        const sectionTitles = {
            verdict: 'Verdict',
            summary: 'Summary',
            decisionSurface: 'Decision surface',
            callouts: 'Callouts',
            verify: 'Verify first',
            keyFindings: 'Key findings',
            diagrams: 'Diagrams',
            findings: 'Findings',
            evidence: 'Evidence',
            tradeoffs: 'Trade-offs',
            risks: 'Risks',
            actionPlan: 'Action plan',
            appendix: 'Appendix'
        };

        const appendText = (parent, text, className = 'text-block') => {
            parent.appendChild(el('div', className, text));
        };

        const appendPillOrText = (td, col, val) => {
            if (val === null || val === undefined) return;
            const normalized = String(val).toLowerCase();
            if (['severity', 'impact', 'kind'].includes(col.toLowerCase())) {
                td.appendChild(el('span', `pill ${normalized}`, String(val)));
            } else {
                td.textContent = String(val);
            }
        };

        const appendTable = (body, columns, rows, labels = {}) => {
            const wrapper = el('div', 'table-wrap');
            const table = el('table', 'data-table');
            const thead = document.createElement('thead');
            const tbody = document.createElement('tbody');
            const trHead = document.createElement('tr');

            columns.forEach(col => trHead.appendChild(el('th', '', labels[col] || titleize(col))));
            thead.appendChild(trHead);

            rows.forEach(row => {
                const tr = document.createElement('tr');
                columns.forEach(col => {
                    const td = document.createElement('td');
                    appendPillOrText(td, col, row[col]);
                    tr.appendChild(td);
                });
                tbody.appendChild(tr);
            });

            table.appendChild(thead);
            table.appendChild(tbody);
            wrapper.appendChild(table);
            body.appendChild(wrapper);
        };

        const renderKeyValue = (value, body) => {
            const kvList = el('div', 'key-value-list');
            Object.entries(value).forEach(([k, v]) => {
                kvList.appendChild(el('div', 'key-value-key', titleize(k)));
                kvList.appendChild(el('div', 'key-value-value', String(v)));
            });
            body.appendChild(kvList);
        };

        // Shape-based fallback renderer: infers the visual from the value's JS
        // shape (string -> text, array of objects -> table, array of scalars ->
        // list, object -> key-value). Named renderers below override this for
        // cognitive-load-critical components.
        const renderByShape = (value, body) => {
            if (typeof value === 'string') {
                appendText(body, value);
            } else if (Array.isArray(value)) {
                if (value.length === 0) {
                    appendText(body, 'None');
                } else if (typeof value[0] === 'object') {
                    const columns = [...new Set(value.flatMap(row => Object.keys(row || {})))];
                    appendTable(body, columns, value);
                } else {
                    const ul = document.createElement('ul');
                    ul.className = 'scalar-list';
                    value.forEach(item => ul.appendChild(el('li', '', String(item))));
                    body.appendChild(ul);
                }
            } else if (typeof value === 'object' && value !== null) {
                renderKeyValue(value, body);
            } else {
                appendText(body, String(value));
            }
        };

        const renderVerify = (items, body) => {
            const lead = el('p', 'section-lead', 'Inspect these first: each row names a likely weak spot, why it matters, and the exact check to run.');
            body.appendChild(lead);
            appendTable(body, ['what', 'why', 'check'], items, {
                what: 'What to review',
                why: 'Why it is risky',
                check: 'How to verify'
            });
        };

        const renderCallouts = (items, body) => {
            const wrap = el('div', 'callout-list');
            items.forEach(item => {
                const kind = String(item.kind || 'note').toLowerCase();
                const card = el('div', `callout ${kind}`);
                const defaultLabel = kind === 'decision' ? 'Decision' : kind === 'warning' ? 'Warning' : 'Note';
                card.appendChild(el('div', 'callout-label', item.label || defaultLabel));
                card.appendChild(el('div', 'callout-text', item.text || ''));
                wrap.appendChild(card);
            });
            body.appendChild(wrap);
        };

        const renderAppendix = (value, body) => {
            const details = document.createElement('details');
            details.className = 'appendix-details';
            const summary = el('summary', '', 'Show exhaustive detail');
            details.appendChild(summary);
            appendText(details, value);
            body.appendChild(details);
        };

        const renderDiagrams = (items, body) => {
            const lead = el('p', 'section-lead', 'Small diagrams reduce mental simulation for flows, dependencies, state, sequence, and timelines.');
            body.appendChild(lead);
            items.forEach((item, index) => {
                const figure = el('figure', 'diagram-card');
                figure.appendChild(el('figcaption', 'diagram-title', item.title || `Diagram ${index + 1}`));

                if ((item.type || 'mermaid') === 'mermaid') {
                    const diagram = el('div', 'mermaid diagram-render', item.code || '');
                    figure.appendChild(diagram);

                    const source = document.createElement('details');
                    source.className = 'diagram-source';
                    source.appendChild(el('summary', '', 'Show Mermaid source'));
                    source.appendChild(el('pre', '', item.code || ''));
                    figure.appendChild(source);
                } else {
                    figure.appendChild(el('pre', 'diagram-fallback', item.code || ''));
                }

                body.appendChild(figure);
            });
        };

        const renderMermaidDiagrams = () => {
            const nodes = reportContent.querySelectorAll('.mermaid');
            if (nodes.length === 0 || !window.mermaid) return;

            try {
                window.mermaid.initialize({
                    startOnLoad: false,
                    securityLevel: 'strict',
                    theme: 'dark'
                });

                if (typeof window.mermaid.run === 'function') {
                    window.mermaid.run({ nodes }).catch(err => {
                        console.error('Mermaid render failed:', err);
                    });
                } else if (typeof window.mermaid.init === 'function') {
                    window.mermaid.init(undefined, nodes);
                }
            } catch (err) {
                console.error('Mermaid initialization failed:', err);
            }
        };

        // Component dispatch seam. report.schema.json is the closed catalog of
        // components; the viewer maps each component name to a renderer.
        const COMPONENT_RENDERERS = {
            decisionSurface: renderKeyValue,
            callouts: renderCallouts,
            verify: renderVerify,
            diagrams: renderDiagrams,
            appendix: renderAppendix
        };

        // COMPONENT_ORDER enforces the cognitive-load "layer-cake" reading order
        // regardless of authoring key order: decision surface first, fragile
        // verification checks near the top, appendix last.
        const COMPONENT_ORDER = [
            'verdict', 'summary', 'decisionSurface', 'callouts', 'verify',
            'keyFindings', 'diagrams', 'findings', 'evidence', 'tradeoffs', 'risks',
            'actionPlan', 'appendix'
        ];

        const skipKeys = new Set(['meta']);
        const present = Object.keys(data).filter(k => !skipKeys.has(k));
        const orderedKeys = [
            ...COMPONENT_ORDER.filter(k => present.includes(k)),
            ...present.filter(k => !COMPONENT_ORDER.includes(k))
        ];

        if (orderedKeys.length > 1) {
            const nav = el('nav', 'report-section-nav');
            orderedKeys.forEach(key => {
                const a = document.createElement('a');
                a.href = `#report-section-${key}`;
                a.textContent = sectionTitles[key] || titleize(key);
                if (key === 'verify') a.className = 'alert';
                nav.appendChild(a);
            });
            reportContent.appendChild(nav);
        }

        orderedKeys.forEach(key => {
            const value = data[key];

            const section = el('section', `section section-${key}`);
            section.id = `report-section-${key}`;
            const header = el('div', 'section-header', sectionTitles[key] || titleize(key));
            const body = el('div', 'section-body');

            section.appendChild(header);
            section.appendChild(body);

            const renderer = COMPONENT_RENDERERS[key] || renderByShape;
            renderer(value, body);

            reportContent.appendChild(section);
        });

        renderMermaidDiagrams();
    }
});
