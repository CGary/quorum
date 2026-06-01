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

        // Safe helper to create elements using textContent
        const el = (tag, className, text) => {
            const element = document.createElement(tag);
            if (className) element.className = className;
            if (text !== undefined) element.textContent = text;
            return element;
        };

        // Shape-based fallback renderer: infers the visual from the value's JS
        // shape (string -> text, array of objects -> table, array of scalars ->
        // list, object -> key-value). This is the default for any component that
        // does not have a named renderer registered below.
        const renderByShape = (value, body) => {
            if (typeof value === 'string') {
                body.appendChild(el('div', 'text-block', value));
            } else if (Array.isArray(value)) {
                if (value.length === 0) {
                    body.appendChild(el('div', 'text-block', 'None'));
                } else if (typeof value[0] === 'object') {
                    // Render as table
                    const table = el('table', 'data-table');
                    const thead = document.createElement('thead');
                    const tbody = document.createElement('tbody');
                    const trHead = document.createElement('tr');

                    const columns = Object.keys(value[0]);
                    columns.forEach(col => {
                        trHead.appendChild(el('th', '', col.replace(/([A-Z])/g, ' $1').trim()));
                    });
                    thead.appendChild(trHead);

                    value.forEach(row => {
                        const tr = document.createElement('tr');
                        columns.forEach(col => {
                            const td = document.createElement('td');
                            const val = row[col];
                            if (col.toLowerCase() === 'severity' || col.toLowerCase() === 'impact') {
                                const pill = el('span', `pill ${String(val).toLowerCase()}`, String(val));
                                td.appendChild(pill);
                            } else {
                                td.textContent = val !== null && val !== undefined ? String(val) : '';
                            }
                            tr.appendChild(td);
                        });
                        tbody.appendChild(tr);
                    });

                    table.appendChild(thead);
                    table.appendChild(tbody);
                    body.appendChild(table);
                } else {
                    const ul = document.createElement('ul');
                    ul.style.listStylePosition = 'inside';
                    ul.style.fontSize = '0.875rem';
                    value.forEach(item => {
                        ul.appendChild(el('li', '', String(item)));
                    });
                    body.appendChild(ul);
                }
            } else if (typeof value === 'object' && value !== null) {
                const kvList = el('div', 'key-value-list');
                Object.entries(value).forEach(([k, v]) => {
                    kvList.appendChild(el('div', 'key-value-key', k.replace(/([A-Z])/g, ' $1').trim()));
                    kvList.appendChild(el('div', 'key-value-value', String(v)));
                });
                body.appendChild(kvList);
            } else {
                body.appendChild(el('div', 'text-block', String(value)));
            }
        };

        // Component dispatch seam. report.schema.json is the closed catalog of
        // components; the viewer maps each component name to a renderer. Named
        // renderers are added additively (e.g. a future `diagram` paired with a
        // meta.schemaVersion bump); any component without one falls back to
        // renderByShape, so every catalog entry always has a visual.
        const COMPONENT_RENDERERS = {
            // key -> (value, body) => void. Empty today: all components render
            // via renderByShape. New named renderers slot in here.
        };

        // COMPONENT_ORDER enforces the cognitive-load "layer-cake" reading order
        // (verdict first, appendix last) regardless of the authoring key order.
        // Components not listed here render after the known ones, in payload order.
        const COMPONENT_ORDER = [
            'verdict', 'summary', 'decisionSurface', 'keyFindings',
            'findings', 'evidence', 'tradeoffs', 'risks', 'actionPlan', 'appendix'
        ];

        const skipKeys = new Set(['meta']);
        const present = Object.keys(data).filter(k => !skipKeys.has(k));
        const orderedKeys = [
            ...COMPONENT_ORDER.filter(k => present.includes(k)),
            ...present.filter(k => !COMPONENT_ORDER.includes(k))
        ];

        orderedKeys.forEach(key => {
            const value = data[key];

            const section = el('div', 'section');
            const header = el('div', 'section-header', key.replace(/([A-Z])/g, ' $1').trim());
            const body = el('div', 'section-body');

            section.appendChild(header);
            section.appendChild(body);

            const renderer = COMPONENT_RENDERERS[key] || renderByShape;
            renderer(value, body);

            reportContent.appendChild(section);
        });
    }
});
