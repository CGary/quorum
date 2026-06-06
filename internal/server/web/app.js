document.addEventListener('DOMContentLoaded', () => {
    const projectSelect = document.getElementById('project-select');
    const reportList = document.getElementById('report-list');
    const memoryList = document.getElementById('memory-list');
    const taskList = document.getElementById('task-list');
    const reportsTab = document.getElementById('reports-tab');
    const memoriesTab = document.getElementById('memories-tab');
    const tasksTab = document.getElementById('tasks-tab');
    const memoryControls = document.getElementById('memory-controls');
    const taskControls = document.getElementById('task-controls');
    const memoryType = document.getElementById('memory-type');
    const memorySearch = document.getElementById('memory-search');
    const taskLocation = document.getElementById('task-location');
    const taskSearch = document.getElementById('task-search');
    const reportTitle = document.getElementById('report-title');
    const reportDate = document.getElementById('report-date');
    const reportContent = document.getElementById('report-content');

    let currentProject = '';
    let activeView = 'reports';
    let memorySearchTimer = null;
    let taskSearchTimer = null;

    const makeEl = (tag, className, text) => {
        const element = document.createElement(tag);
        if (className) element.className = className;
        if (text !== undefined) element.textContent = text;
        return element;
    };

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
        let title = 'No Report Selected';
        if (activeView === 'memories') title = 'No Memory Selected';
        if (activeView === 'tasks') title = 'No Task Selected';
        clearContent(title);
        if (activeView === 'reports') {
            loadReports(currentProject);
        } else if (activeView === 'memories') {
            loadMemories(currentProject);
        } else {
            loadTasks(currentProject);
        }
    });

    reportsTab.addEventListener('click', () => switchView('reports'));
    memoriesTab.addEventListener('click', () => switchView('memories'));
    tasksTab.addEventListener('click', () => switchView('tasks'));
    memoryType.addEventListener('change', () => currentProject && loadMemories(currentProject));
    memorySearch.addEventListener('input', () => {
        clearTimeout(memorySearchTimer);
        memorySearchTimer = setTimeout(() => currentProject && loadMemories(currentProject), 200);
    });
    taskLocation.addEventListener('change', () => currentProject && loadTasks(currentProject));
    taskSearch.addEventListener('input', () => {
        clearTimeout(taskSearchTimer);
        taskSearchTimer = setTimeout(() => currentProject && loadTasks(currentProject), 200);
    });

    function switchView(view) {
        activeView = view;
        const reportsActive = view === 'reports';
        const memoriesActive = view === 'memories';
        const tasksActive = view === 'tasks';

        reportsTab.classList.toggle('active', reportsActive);
        memoriesTab.classList.toggle('active', memoriesActive);
        tasksTab.classList.toggle('active', tasksActive);

        reportsTab.setAttribute('aria-selected', reportsActive ? 'true' : 'false');
        memoriesTab.setAttribute('aria-selected', memoriesActive ? 'true' : 'false');
        tasksTab.setAttribute('aria-selected', tasksActive ? 'true' : 'false');

        reportList.classList.toggle('hidden', !reportsActive);
        memoryList.classList.toggle('hidden', !memoriesActive);
        taskList.classList.toggle('hidden', !tasksActive);

        memoryControls.classList.toggle('hidden', !memoriesActive);
        taskControls.classList.toggle('hidden', !tasksActive);

        let title = 'No Report Selected';
        if (memoriesActive) title = 'No Memory Selected';
        if (tasksActive) title = 'No Task Selected';
        clearContent(title);

        if (!currentProject) return;
        if (reportsActive) {
            loadReports(currentProject);
        } else if (memoriesActive) {
            loadMemories(currentProject);
        } else {
            loadTasks(currentProject);
        }
    }

    function clearContent(title) {
        reportTitle.textContent = title;
        reportDate.textContent = '';
        let typeLabel = 'report';
        if (activeView === 'memories') typeLabel = 'memory';
        if (activeView === 'tasks') typeLabel = 'task';
        reportContent.innerHTML = `<div class="empty-state"><p>Select a ${typeLabel} from the sidebar to view its contents.</p></div>`;
    }

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

    function loadMemories(projectId) {
        memoryList.innerHTML = '<div class="empty-state">Loading memories...</div>';
        const params = new URLSearchParams();
        if (memoryType.value) params.set('type', memoryType.value);
        const q = memorySearch.value.trim();
        if (q) params.set('q', q);
        const suffix = params.toString() ? `?${params.toString()}` : '';
        fetch(`/api/projects/${projectId}/memories${suffix}`)
            .then(res => {
                if (!res.ok) throw new Error('Failed to load memories');
                return res.json();
            })
            .then(data => {
                memoryList.innerHTML = '';
                const counts = data.counts || {};
                const summary = makeEl('div', 'memory-counts', `Decisions ${counts.decision || 0} · Patterns ${counts.pattern || 0} · Lessons ${counts.lesson || 0}`);
                memoryList.appendChild(summary);
                const items = data.items || [];
                if (items.length === 0) {
                    memoryList.appendChild(makeEl('div', 'empty-state', 'No memories found'));
                    return;
                }
                items.forEach(m => {
                    const item = makeEl('div', 'report-item memory-item');
                    const title = makeEl('div', 'report-item-title', m.title || m.id);
                    const meta = makeEl('div', 'report-item-date', `${m.type} · ${m.source_task || 'no task'} · ${m.created_at || ''}`);
                    const excerpt = makeEl('div', 'memory-excerpt', m.content_excerpt || '');
                    item.appendChild(title);
                    item.appendChild(meta);
                    item.appendChild(excerpt);
                    item.addEventListener('click', () => {
                        document.querySelectorAll('.report-item').forEach(el => el.classList.remove('active'));
                        item.classList.add('active');
                        loadMemoryDetail(projectId, m.id);
                    });
                    memoryList.appendChild(item);
                });
            })
            .catch(err => {
                console.error('Error fetching memories:', err);
                memoryList.innerHTML = '<div class="empty-state">Error loading memories</div>';
            });
    }

    function loadMemoryDetail(projectId, memoryId) {
        reportContent.innerHTML = '<div class="empty-state">Loading memory...</div>';
        reportTitle.textContent = `Loading ${memoryId}...`;
        reportDate.textContent = '';
        fetch(`/api/projects/${projectId}/memories/${memoryId}`)
            .then(res => {
                if (!res.ok) throw new Error('Failed to load memory');
                return res.json();
            })
            .then(renderMemory)
            .catch(err => {
                console.error('Error fetching memory detail:', err);
                reportTitle.textContent = 'Error';
                reportContent.innerHTML = '<div class="empty-state">Failed to load memory details.</div>';
            });
    }

    function renderMemory(memory) {
        reportTitle.textContent = memory.title || memory.id;
        reportDate.textContent = memory.created_at || '';
        reportContent.innerHTML = '';

        const header = makeEl('div', 'memory-header-wrap');
        header.appendChild(makeEl('div', `pill ${memory.type}`, memory.type || 'memory'));
        header.appendChild(makeEl('h1', 'report-main-title', memory.title || memory.id));
        header.appendChild(makeEl('div', 'report-kicker', `${memory.id} · ${memory.source_task || 'no source task'}`));
        if (memory.context) header.appendChild(makeEl('div', 'report-summary-prosa', memory.context));
        reportContent.appendChild(header);

        const contentSection = makeEl('section', 'section section-memory');
        contentSection.appendChild(makeEl('div', 'section-header', 'Curated Content'));
        const contentBody = makeEl('div', 'section-body');
        contentBody.appendChild(makeEl('div', 'text-block', memory.content || ''));
        contentSection.appendChild(contentBody);
        reportContent.appendChild(contentSection);

        const refs = makeEl('section', 'section section-memory');
        refs.appendChild(makeEl('div', 'section-header', 'References'));
        const body = makeEl('div', 'section-body');
        const kv = makeEl('div', 'key-value-list');
        const appendKV = (key, value) => {
            kv.appendChild(makeEl('div', 'key-value-key', key));
            kv.appendChild(makeEl('div', 'key-value-value', value || 'None'));
        };
        appendKV('Supersedes', memory.supersedes);
        appendKV('Superseded by', (memory.superseded_by || []).join(', '));
        appendKV('Related', (memory.related || []).join(', '));
        appendKV('Anti-patterns', (memory.anti_patterns || []).join('\n'));
        body.appendChild(kv);
        refs.appendChild(body);
        reportContent.appendChild(refs);
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

    function loadTasks(projectId) {
        taskList.innerHTML = '<div class="empty-state">Loading tasks...</div>';

        const loc = taskLocation.value;
        const q = taskSearch.value;

        let url = `/api/projects/${projectId}/tasks?`;
        if (loc) url += `location=${encodeURIComponent(loc)}&`;
        if (q) url += `q=${encodeURIComponent(q)}&`;

        fetch(url)
            .then(res => res.json())
            .then(data => {
                taskList.innerHTML = '';
                const tasks = data.items || [];
                if (tasks.length > 0) {
                    tasks.forEach(t => {
                        const item = document.createElement('div');
                        item.className = 'report-item';

                        const titleWrap = document.createElement('div');
                        titleWrap.style.display = 'flex';
                        titleWrap.style.justifyContent = 'space-between';
                        titleWrap.style.alignItems = 'center';

                        const title = document.createElement('div');
                        title.className = 'report-item-title';
                        title.textContent = t.id;

                        const pill = document.createElement('span');
                        pill.className = `pill ${t.location}`;
                        pill.textContent = t.location;

                        titleWrap.appendChild(title);
                        titleWrap.appendChild(pill);

                        const summary = document.createElement('div');
                        summary.className = 'report-item-date';
                        summary.textContent = t.summary || 'No summary';
                        summary.style.whiteSpace = 'nowrap';
                        summary.style.overflow = 'hidden';
                        summary.style.textOverflow = 'ellipsis';
                        summary.style.marginTop = '4px';

                        item.appendChild(titleWrap);
                        item.appendChild(summary);

                        item.addEventListener('click', () => {
                            document.querySelectorAll('.report-item').forEach(el => el.classList.remove('active'));
                            item.classList.add('active');
                            loadTaskDetail(projectId, t.id);
                        });

                        taskList.appendChild(item);
                    });
                } else {
                    taskList.innerHTML = '<div class="empty-state">No tasks found</div>';
                }
            })
            .catch(err => {
                console.error('Error fetching tasks:', err);
                taskList.innerHTML = '<div class="empty-state">Error loading tasks</div>';
            });
    }

    function loadTaskDetail(projectId, taskId) {
        reportContent.innerHTML = '<div class="empty-state">Loading task...</div>';
        reportTitle.textContent = `Loading ${taskId}...`;
        reportDate.textContent = '';

        fetch(`/api/projects/${projectId}/tasks/${taskId}`)
            .then(res => {
                if (!res.ok) throw new Error('Failed to load task details');
                return res.json();
            })
            .then(data => {
                renderTask(data);
            })
            .catch(err => {
                console.error('Error fetching task details:', err);
                reportTitle.textContent = 'Error';
                reportContent.innerHTML = '<div class="empty-state">Failed to load task details.</div>';
            });
    }

    function renderTask(task) {
        reportTitle.textContent = task.id;
        const d = new Date(task.updated_at);
        reportDate.textContent = isNaN(d.getTime()) ? task.updated_at : d.toLocaleString();
        reportContent.innerHTML = '';

        // Header
        const header = makeEl('div', 'report-header-wrap');
        const topRow = document.createElement('div');
        topRow.style.display = 'flex';
        topRow.style.alignItems = 'center';
        topRow.style.gap = '1rem';

        topRow.appendChild(makeEl('div', `pill ${task.location}`, task.location));
        if (task.risk) {
            topRow.appendChild(makeEl('div', `pill ${task.risk}`, `Risk: ${task.risk}`));
        }
        header.appendChild(topRow);

        header.appendChild(makeEl('h1', 'report-main-title', task.id));
        header.appendChild(makeEl('div', 'report-kicker', `Directory: ${task.directory}`));

        if (task.summary) {
            header.appendChild(makeEl('div', 'report-summary-prosa', task.summary));
        }
        reportContent.appendChild(header);

        // Metadata section
        const metaSection = makeEl('section', 'section');
        metaSection.appendChild(makeEl('div', 'section-header', 'Task Metadata'));
        const metaBody = makeEl('div', 'section-body');
        const metaKV = makeEl('div', 'key-value-list');
        const appendMetaKV = (key, value) => {
            metaKV.appendChild(makeEl('div', 'key-value-key', key));
            metaKV.appendChild(makeEl('div', 'key-value-value', value || 'None'));
        };

        appendMetaKV('Worktree Present', task.worktree_present ? 'Yes' : 'No');
        if (task.parent_task) {
            appendMetaKV('Parent Task', task.parent_task);
        }
        if (task.parent_state) {
            appendMetaKV('Parent State', task.parent_state);
        }
        metaBody.appendChild(metaKV);
        metaSection.appendChild(metaBody);
        reportContent.appendChild(metaSection);

        // Children tasks section if applicable
        if (task.children && task.children.length > 0) {
            const childrenSection = makeEl('section', 'section');
            childrenSection.appendChild(makeEl('div', 'section-header', 'Decomposition Children'));
            const childrenBody = makeEl('div', 'section-body');
            const ul = document.createElement('ul');
            ul.className = 'bullet-list';
            task.children.forEach(c => {
                const li = document.createElement('li');
                li.innerHTML = `<strong>${c.id}</strong> [${c.location}]: ${c.summary || 'No summary'}`;
                ul.appendChild(li);
            });
            childrenBody.appendChild(ul);
            childrenSection.appendChild(childrenBody);
            reportContent.appendChild(childrenSection);
        }

        // Artifacts checklist section
        const artifactsSection = makeEl('section', 'section');
        artifactsSection.appendChild(makeEl('div', 'section-header', 'Lifecycle Artifacts Status'));
        const artifactsBody = makeEl('div', 'section-body');

        const artTableWrap = makeEl('div', 'table-wrap');
        const artTable = makeEl('table', 'data-table');
        artTable.innerHTML = `
            <thead>
                <tr>
                    <th>Artifact</th>
                    <th>Status</th>
                </tr>
            </thead>
            <tbody>
                ${Object.entries(task.artifacts).map(([name, present]) => `
                    <tr>
                        <td><strong>${name}</strong></td>
                        <td>
                            <span class="pill ${present ? 'done' : 'info'}">
                                ${present ? 'Present ✓' : 'Missing ✗'}
                            </span>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        `;
        artTableWrap.appendChild(artTable);
        artifactsBody.appendChild(artTableWrap);
        artifactsSection.appendChild(artifactsBody);
        reportContent.appendChild(artifactsSection);

        // Suggestions / Manual Actions section
        const suggSection = makeEl('section', 'section');
        suggSection.appendChild(makeEl('div', 'section-header', 'Suggested Actions'));
        const suggBody = makeEl('div', 'section-body');
        suggBody.innerHTML = `
            <div class="verdict-box">
                <div class="verdict-text">Manual CLI Commands</div>
                <div class="verdict-confidence">
                    <pre style="margin: 0.5rem 0 0 0; font-family: monospace; color: #cbd5e1; background: transparent; border: none; padding: 0;">quorum task status ${task.id}\nquorum task back ${task.id}</pre>
                </div>
            </div>
        `;
        suggSection.appendChild(suggBody);
        reportContent.appendChild(suggSection);

        // Render detailed/summarized artifacts
        if (task.spec) {
            renderCollapsibleArtifact('00-spec.yaml', task.spec);
        }
        if (task.blueprint) {
            renderCollapsibleArtifact('01-blueprint.yaml', task.blueprint);
        }
        if (task.contract) {
            renderCollapsibleContract(task.contract);
        }
        if (task.implementation_log) {
            renderCollapsibleArtifact('04-implementation-log.yaml', task.implementation_log);
        }
        if (task.validation) {
            renderCollapsibleArtifact('05-validation.json', task.validation);
        }
        if (task.review) {
            renderCollapsibleArtifact('06-review.json', task.review);
        }
        if (task.trace) {
            renderCollapsibleTrace(task.trace);
        }
        if (task.feedback) {
            renderCollapsibleArtifact('feedback.json', task.feedback);
        }
    }

    function renderCollapsibleArtifact(name, data) {
        const details = makeEl('details', 'artifact-details');
        const summary = makeEl('summary', '', name);
        const pre = document.createElement('pre');
        const code = document.createElement('code');

        code.textContent = JSON.stringify(data, null, 2);

        pre.appendChild(code);
        details.appendChild(summary);
        details.appendChild(pre);
        reportContent.appendChild(details);
    }

    function renderCollapsibleContract(contract) {
        const details = makeEl('details', 'artifact-details');
        const summary = makeEl('summary', '', '02-contract.yaml (summarized)');
        const content = makeEl('div', 'section-body');
        content.style.padding = '1rem';

        const kv = makeEl('div', 'key-value-list');
        const appendKV = (key, value) => {
            kv.appendChild(makeEl('div', 'key-value-key', key));
            kv.appendChild(makeEl('div', 'key-value-value', value || 'None'));
        };

        appendKV('Contract Summary', contract.summary);
        appendKV('Contract Goal', contract.goal);
        appendKV('Touch Paths', contract.touch && contract.touch.length > 0 ? contract.touch.join('\n') : 'None');
        appendKV('Verify Commands', contract.verify_commands && contract.verify_commands.length > 0 ? contract.verify_commands.join('\n') : 'None');

        content.appendChild(kv);
        details.appendChild(summary);
        details.appendChild(content);
        reportContent.appendChild(details);
    }

    function renderCollapsibleTrace(trace) {
        const details = makeEl('details', 'artifact-details');
        const summary = makeEl('summary', '', '07-trace.json (summarized)');
        const content = makeEl('div', 'section-body');
        content.style.padding = '1rem';

        const kv = makeEl('div', 'key-value-list');
        const appendKV = (key, value) => {
            kv.appendChild(makeEl('div', 'key-value-key', key));
            kv.appendChild(makeEl('div', 'key-value-value', value || 'None'));
        };

        appendKV('Trace Summary', trace.summary);
        appendKV('Attempts Count', String(trace.attempts_count));
        appendKV('Total Cost USD', trace.total_cost_usd !== undefined ? `$${trace.total_cost_usd.toFixed(4)}` : '$0.0000');

        if (trace.last_attempt) {
            appendKV('Last Attempt', JSON.stringify(trace.last_attempt, null, 2));
        }

        content.appendChild(kv);
        details.appendChild(summary);
        details.appendChild(content);
        reportContent.appendChild(details);
    }
});
