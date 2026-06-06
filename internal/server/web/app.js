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

        reportContent.innerHTML = '';

        // Shared helpers
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

        // --- SEMANTIC RENDERER ---
        const renderSemanticReport = () => {
            const content = data.content;
            const presentation = data.presentation || {};
            const profile = presentation.profile || 'cognitive';

            // 1. Render Header (title, kicker, verdict, summary)
            renderSemanticHeader(content, presentation);

            // 2. Order sections according to presentation.profile
            const orderedSections = orderSections(content.sections || [], profile);

            // 3. Render Navigation
            if (orderedSections.length > 1) {
                const nav = el('nav', 'report-section-nav');
                orderedSections.forEach(sec => {
                    const a = document.createElement('a');
                    a.href = `#report-section-${sec.id}`;
                    a.textContent = sec.title;
                    if (sec.role === 'verification') a.className = 'alert';
                    nav.appendChild(a);
                });
                reportContent.appendChild(nav);
            }

            // 4. Render each section
            orderedSections.forEach(sec => {
                const section = el('section', `section section-${sec.role}`);
                section.id = `report-section-${sec.id}`;
                const header = el('div', 'section-header', sec.title);
                const body = el('div', 'section-body');

                section.appendChild(header);
                section.appendChild(body);

                renderSemanticSection(sec, body, presentation);

                reportContent.appendChild(section);
            });

            renderMermaidDiagrams();
        };

        const renderSemanticHeader = (content, presentation) => {
            const headerWrap = el('div', 'report-header-wrap');
            if (content.kicker) {
                headerWrap.appendChild(el('div', 'report-kicker', content.kicker));
            }
            headerWrap.appendChild(el('h1', 'report-main-title', content.title));

            if (content.verdict) {
                const confClass = content.verdict.confidence ? `confidence-${content.verdict.confidence.toLowerCase()}` : '';
                const verdictBox = el('div', `verdict-box ${confClass}`);
                verdictBox.appendChild(el('div', 'verdict-text', content.verdict.text));
                if (content.verdict.confidence) {
                    verdictBox.appendChild(el('span', 'verdict-confidence', `Confidence: ${content.verdict.confidence}`));
                }
                headerWrap.appendChild(verdictBox);
            }

            if (content.summary) {
                headerWrap.appendChild(el('div', 'report-summary-prosa', content.summary));
            }

            reportContent.appendChild(headerWrap);
        };

        // PROFILE_ORDER definition
        const PROFILE_ORDER = {
            cognitive: [
                'decision_surface', 'verification', 'callout', 'findings',
                'diagram', 'analysis', 'tradeoffs', 'risks',
                'action_plan', 'evidence', 'metrics', 'appendix'
            ],
            executive: [
                'decision_surface', 'risks', 'tradeoffs', 'action_plan'
            ],
            audit: [
                'verification', 'findings', 'evidence', 'risks',
                'action_plan', 'appendix'
            ],
            teaching: [
                'diagram', 'analysis', 'action_plan', 'verification', 'appendix'
            ],
            raw: [] // preserves authoring order
        };

        const orderSections = (sections, profile) => {
            const order = PROFILE_ORDER[profile] || PROFILE_ORDER.cognitive;
            if (profile === 'raw' || order.length === 0) {
                return [...sections];
            }

            // Sort logic: stable sort placing non-enumerated roles at the end in authoring order
            const mapped = sections.map((sec, idx) => ({ sec, idx }));
            mapped.sort((a, b) => {
                const idxA = order.indexOf(a.sec.role);
                const idxB = order.indexOf(b.sec.role);

                const hasA = idxA !== -1;
                const hasB = idxB !== -1;

                if (hasA && hasB) {
                    if (idxA !== idxB) return idxA - idxB;
                    return a.idx - b.idx; // Keep author order for same role
                }
                if (hasA && !hasB) return -1;
                if (!hasA && hasB) return 1;
                return a.idx - b.idx; // Keep author order for both non-enumerated
            });

            return mapped.map(item => item.sec);
        };

        const renderSemanticSection = (sec, body, presentation) => {
            const density = presentation.density || 'medium';

            switch (sec.role) {
                case 'decision_surface':
                    if (sec.body) {
                        renderKeyValue(sec.body, body);
                    }
                    break;

                case 'verification':
                    if (sec.items) {
                        const items = Array.isArray(sec.items) ? sec.items : [];
                        renderVerify(items, body);
                    }
                    break;

                case 'findings':
                    if (sec.items) {
                        appendTable(body, ['id', 'finding', 'why', 'action', 'severity'], sec.items, {
                            id: 'ID',
                            finding: 'Finding',
                            why: 'Why it matters',
                            action: 'Recommended action',
                            severity: 'Severity'
                        });
                    }
                    break;

                case 'analysis':
                    if (sec.body) {
                        appendText(body, sec.body);
                    }
                    if (sec.details && sec.details.body) {
                        const details = document.createElement('details');
                        details.className = 'analysis-details-element';
                        // Collapse if density != high
                        if (density === 'high') {
                            details.open = true;
                        }
                        const summary = el('summary', '', sec.details.label || 'Show details');
                        details.appendChild(summary);
                        appendText(details, sec.details.body);
                        body.appendChild(details);
                    }
                    break;

                case 'diagram':
                    if (sec.diagram) {
                        const title = sec.title || 'Diagram';
                        const figure = el('figure', 'diagram-card');
                        if (sec.diagram.type === 'mermaid') {
                            const diagRender = el('div', 'mermaid diagram-render', sec.diagram.code || '');
                            figure.appendChild(diagRender);

                            const source = document.createElement('details');
                            source.className = 'diagram-source';
                            source.appendChild(el('summary', '', 'Show Mermaid source'));
                            source.appendChild(el('pre', '', sec.diagram.code || ''));
                            figure.appendChild(source);
                        } else {
                            figure.appendChild(el('pre', 'diagram-fallback', sec.diagram.code || ''));
                        }
                        body.appendChild(figure);
                    }
                    break;

                case 'tradeoffs':
                    if (sec.items) {
                        appendTable(body, ['option', 'upside', 'downside', 'useWhen', 'avoidWhen'], sec.items, {
                            option: 'Option',
                            upside: 'Upside',
                            downside: 'Downside',
                            useWhen: 'Use when',
                            avoidWhen: 'Avoid when'
                        });
                    }
                    break;

                case 'risks':
                    if (sec.items) {
                        appendTable(body, ['risk', 'signal', 'impact', 'mitigation'], sec.items, {
                            risk: 'Risk',
                            signal: 'Risk signal',
                            impact: 'Impact',
                            mitigation: 'Mitigation'
                        });
                    }
                    break;

                case 'action_plan':
                    if (sec.items) {
                        appendTable(body, ['step', 'action', 'owner'], sec.items, {
                            step: 'Step',
                            action: 'Action',
                            owner: 'Owner'
                        });
                    }
                    break;

                case 'evidence':
                    if (sec.items) {
                        // Render with links if possible, or table
                        appendTable(body, ['findingId', 'path', 'details'], sec.items, {
                            findingId: 'Finding ID',
                            path: 'Path',
                            details: 'Details'
                        });
                    }
                    break;

                case 'appendix':
                    if (sec.body) {
                        const details = document.createElement('details');
                        details.className = 'appendix-details';
                        const summary = el('summary', '', 'Show exhaustive detail');
                        details.appendChild(summary);
                        appendText(details, sec.body);
                        body.appendChild(details);
                    }
                    break;

                case 'metrics':
                    if (sec.items) {
                        appendTable(body, ['label', 'value', 'unit', 'display'], sec.items, {
                            label: 'Metric',
                            value: 'Value',
                            unit: 'Unit',
                            display: 'Visual'
                        });
                    }
                    break;

                case 'callout':
                    if (sec.body) {
                        const kind = String(sec.kind || 'note').toLowerCase();
                        const card = el('div', `callout ${kind}`);
                        card.appendChild(el('div', 'callout-label', sec.label || kind.toUpperCase()));
                        card.appendChild(el('div', 'callout-text', sec.body));
                        body.appendChild(card);
                    }
                    break;

                default:
                    renderByShape(sec, body);
            }
        };

        renderSemanticReport();
    }
});
