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

                        const arts = t.artifacts || {};
                        const total = Object.keys(arts).length;
                        if (total > 0) {
                            const present = Object.values(arts).filter(Boolean).length;
                            const meta = document.createElement('div');
                            meta.className = 'task-item-meta';
                            meta.textContent = `${present}/${total} artifacts${t.worktree_present ? ' · worktree' : ''}`;
                            item.appendChild(meta);
                        }

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

    // Task artifact rendering: structured display layer over lifecycle artifacts.
    // Preserves 100% of the payload (raw JSON appendix per artifact) while the
    // primary path is scannable: lifecycle pipeline, verdict pills, shape-based
    // tables/key-value grids, and progressive disclosure for dense detail.

    const TASK_LIFECYCLE_STEPS = [
        ['00-spec.yaml', 'Spec'],
        ['01-blueprint.yaml', 'Blueprint'],
        ['02-contract.yaml', 'Contract'],
        ['04-implementation-log.yaml', 'Implement'],
        ['05-validation.json', 'Validate'],
        ['06-review.json', 'Review'],
        ['07-trace.json', 'Trace']
    ];

    const TASK_PILL_KEYS = [
        'risk', 'severity', 'verdict', 'overall_result', 'error_category',
        'functional_risk', 'location', 'impact', 'kind', 'status', 'result',
        'parent_state'
    ];

    function taskPillClass(value) {
        const map = {
            passed: 'done', approve: 'done', approved: 'done', done: 'done', completed: 'done',
            failed: 'failed', reject: 'failed', rejected: 'failed', blocked: 'failed',
            revise: 'medium', medium: 'medium', partial: 'medium', pending: 'warning',
            high: 'high', critical: 'critical', low: 'low',
            inbox: 'inbox', active: 'active'
        };
        return map[String(value).toLowerCase()] || 'info';
    }

    function taskTitleize(key) {
        return String(key)
            .replace(/[_-]+/g, ' ')
            .replace(/\b\w/g, c => c.toUpperCase());
    }

    function appendTaskScalar(parent, key, val) {
        if (val === null || val === undefined || val === '') {
            parent.appendChild(makeEl('span', 'task-muted', 'None'));
            return;
        }
        if (typeof val === 'boolean') {
            parent.textContent = val ? 'Yes' : 'No';
            return;
        }
        if (TASK_PILL_KEYS.includes(String(key).toLowerCase())) {
            parent.appendChild(makeEl('span', `pill ${taskPillClass(val)}`, String(val)));
            return;
        }
        parent.textContent = String(val);
    }

    function renderTaskTable(rows, parent) {
        const columns = [...new Set(rows.flatMap(r => Object.keys(r || {})))];
        const wrap = makeEl('div', 'table-wrap');
        const table = makeEl('table', 'data-table task-table');
        const thead = document.createElement('thead');
        const trHead = document.createElement('tr');
        columns.forEach(c => trHead.appendChild(makeEl('th', '', taskTitleize(c))));
        thead.appendChild(trHead);
        const tbody = document.createElement('tbody');

        rows.forEach(row => {
            const tr = document.createElement('tr');
            columns.forEach(c => {
                const td = document.createElement('td');
                const v = row ? row[c] : undefined;
                if (v === null || v === undefined) {
                    // leave empty: blank cells scan faster than placeholder noise
                } else if (Array.isArray(v)) {
                    if (v.length > 0 && typeof v[0] === 'object' && v[0] !== null) {
                        renderTaskTable(v, td);
                    } else {
                        const ul = makeEl('ul', 'cell-list');
                        v.forEach(item => ul.appendChild(makeEl('li', '', String(item))));
                        td.appendChild(ul);
                    }
                } else if (typeof v === 'object') {
                    renderTaskShape(v, td);
                } else if (c === 'exit_code') {
                    td.appendChild(makeEl('span', `pill ${Number(v) === 0 ? 'done' : 'failed'}`, String(v)));
                } else if (typeof v === 'string' && (v.length > 160 || v.includes('\n'))) {
                    const excerpt = makeEl('details', 'cell-excerpt');
                    excerpt.appendChild(makeEl('summary', '', v.split('\n')[0].slice(0, 80) + '…'));
                    excerpt.appendChild(makeEl('pre', '', v));
                    td.appendChild(excerpt);
                } else {
                    appendTaskScalar(td, c, v);
                }
                tr.appendChild(td);
            });
            tbody.appendChild(tr);
        });

        table.appendChild(thead);
        table.appendChild(tbody);
        wrap.appendChild(table);
        parent.appendChild(wrap);
    }

    function renderTaskShape(value, parent) {
        if (value === null || value === undefined) return;

        if (Array.isArray(value)) {
            if (value.length === 0) {
                parent.appendChild(makeEl('div', 'task-muted', 'None'));
            } else if (typeof value[0] === 'object' && value[0] !== null) {
                renderTaskTable(value, parent);
            } else {
                const ul = makeEl('ul', 'scalar-list');
                value.forEach(item => ul.appendChild(makeEl('li', '', String(item))));
                parent.appendChild(ul);
            }
            return;
        }

        if (typeof value === 'object') {
            // Front-load prose: summary and goal read as lead text, not grid rows.
            const leadKeys = ['summary', 'goal'].filter(k => typeof value[k] === 'string' && value[k]);
            leadKeys.forEach(k => {
                const lead = makeEl('div', 'artifact-lead');
                lead.appendChild(makeEl('div', 'artifact-lead-label', taskTitleize(k)));
                lead.appendChild(makeEl('div', 'artifact-lead-text', value[k]));
                parent.appendChild(lead);
            });

            const scalarKeys = [];
            const complexKeys = [];
            Object.keys(value).forEach(k => {
                if (leadKeys.includes(k)) return;
                const v = value[k];
                if (v !== null && typeof v === 'object') complexKeys.push(k);
                else scalarKeys.push(k);
            });

            if (scalarKeys.length > 0) {
                const kv = makeEl('div', 'key-value-list');
                scalarKeys.forEach(k => {
                    kv.appendChild(makeEl('div', 'key-value-key', taskTitleize(k)));
                    const vEl = makeEl('div', 'key-value-value');
                    appendTaskScalar(vEl, k, value[k]);
                    kv.appendChild(vEl);
                });
                parent.appendChild(kv);
            }

            complexKeys.forEach(k => {
                const block = makeEl('div', 'artifact-subsection');
                block.appendChild(makeEl('div', 'artifact-subheading', taskTitleize(k)));
                renderTaskShape(value[k], block);
                parent.appendChild(block);
            });
            return;
        }

        parent.appendChild(makeEl('div', 'text-block', String(value)));
    }

    function renderTaskLifecycle(task) {
        const wrap = makeEl('div', 'lifecycle-stepper');
        TASK_LIFECYCLE_STEPS.forEach(([file, label], i) => {
            if (i > 0) wrap.appendChild(makeEl('div', 'lifecycle-connector'));
            const present = !!(task.artifacts && task.artifacts[file]);
            const step = makeEl('div', `lifecycle-step ${present ? 'present' : 'missing'}`);
            step.title = `${file}: ${present ? 'present' : 'missing'}`;
            step.appendChild(makeEl('div', 'lifecycle-dot', present ? '✓' : ''));
            step.appendChild(makeEl('div', 'lifecycle-label', label));
            wrap.appendChild(step);
        });
        return wrap;
    }

    function renderTaskArtifact(name, role, data, opts = {}) {
        if (!data) return;
        const details = makeEl('details', 'artifact-details');
        if (opts.open) details.open = true;

        const summary = makeEl('summary', 'artifact-summary');
        summary.appendChild(makeEl('span', 'artifact-name', name));
        summary.appendChild(makeEl('span', 'artifact-role', role));
        if (opts.status) {
            summary.appendChild(makeEl('span', `pill ${taskPillClass(opts.status)}`, String(opts.status)));
        }
        if (opts.meta) {
            summary.appendChild(makeEl('span', 'artifact-meta', opts.meta));
        }
        details.appendChild(summary);

        const body = makeEl('div', 'artifact-body');
        renderTaskShape(data, body);

        // Appendix layer: the exact payload stays available, never deleted.
        const raw = makeEl('details', 'raw-json');
        raw.appendChild(makeEl('summary', '', 'Raw JSON'));
        const pre = document.createElement('pre');
        pre.appendChild(makeEl('code', '', JSON.stringify(data, null, 2)));
        raw.appendChild(pre);
        body.appendChild(raw);

        details.appendChild(body);
        reportContent.appendChild(details);
    }

    function renderTask(task) {
        reportTitle.textContent = task.id;
        const d = new Date(task.updated_at);
        reportDate.textContent = isNaN(d.getTime()) ? task.updated_at : d.toLocaleString();
        reportContent.innerHTML = '';

        const artifacts = task.artifacts || {};
        const valResult = task.validation && task.validation.overall_result;
        const verdict = task.review && task.review.verdict;
        const hasTrace = artifacts['07-trace.json'] && task.trace;
        const hasContract = artifacts['02-contract.yaml'] && task.contract;

        // Decision surface: every verdict that matters, before any detail.
        const header = makeEl('div', 'report-header-wrap');
        const topRow = makeEl('div', 'task-pill-row');
        topRow.appendChild(makeEl('span', `pill ${task.location}`, task.location));
        if (task.risk) topRow.appendChild(makeEl('span', `pill ${taskPillClass(task.risk)}`, `risk: ${task.risk}`));
        if (valResult) topRow.appendChild(makeEl('span', `pill ${taskPillClass(valResult)}`, `validation: ${valResult}`));
        if (verdict) topRow.appendChild(makeEl('span', `pill ${taskPillClass(verdict)}`, `review: ${verdict}`));
        if (task.parent_state) topRow.appendChild(makeEl('span', `pill ${taskPillClass(task.parent_state)}`, `children: ${task.parent_state}`));
        if (task.feedback) topRow.appendChild(makeEl('span', 'pill warning', 'feedback pending'));
        header.appendChild(topRow);

        header.appendChild(makeEl('h1', 'report-main-title', task.id));
        header.appendChild(makeEl('div', 'report-kicker', `Directory: ${task.directory}`));
        if (task.summary) {
            header.appendChild(makeEl('div', 'report-summary-prosa', task.summary));
        }
        reportContent.appendChild(header);

        // Lifecycle pipeline: spatial state beats a presence/absence table.
        reportContent.appendChild(renderTaskLifecycle(task));

        // Status facts: worktree, lineage, attempts, cost, manual CLI paths.
        const statusSection = makeEl('section', 'section');
        statusSection.appendChild(makeEl('div', 'section-header', 'Task Status'));
        const statusBody = makeEl('div', 'section-body');

        const kv = makeEl('div', 'key-value-list');
        const appendKV = (key, build) => {
            kv.appendChild(makeEl('div', 'key-value-key', key));
            const vEl = makeEl('div', 'key-value-value');
            build(vEl);
            kv.appendChild(vEl);
        };
        appendKV('Worktree', el => { el.textContent = task.worktree_present ? 'Present' : 'Missing'; });
        if (task.goal) appendKV('Goal', el => { el.textContent = task.goal; });
        if (task.parent_task) appendKV('Parent Task', el => { el.textContent = task.parent_task; });
        if (hasTrace) {
            appendKV('Attempts', el => { el.textContent = String(task.trace.attempts_count); });
            if (task.trace.total_cost_usd !== undefined) {
                appendKV('Total Cost', el => { el.textContent = `$${task.trace.total_cost_usd.toFixed(4)}`; });
            }
        }
        statusBody.appendChild(kv);

        if (task.children && task.children.length > 0) {
            const block = makeEl('div', 'artifact-subsection');
            block.appendChild(makeEl('div', 'artifact-subheading', 'Decomposition Children'));
            renderTaskTable(task.children, block);
            statusBody.appendChild(block);
        }

        const cli = makeEl('div', 'task-cli-hint');
        cli.appendChild(makeEl('div', 'artifact-subheading', 'Manual CLI'));
        cli.appendChild(makeEl('pre', '', `quorum task status ${task.id}\nquorum task back ${task.id}`));
        statusBody.appendChild(cli);

        statusSection.appendChild(statusBody);
        reportContent.appendChild(statusSection);

        // Artifact sections: collapsed but self-describing; problem-bearing
        // artifacts (failed validation, non-approve review, feedback) open.
        renderTaskArtifact('00-spec.yaml', 'Intent', task.spec, {
            status: task.spec && task.spec.risk ? task.spec.risk : '',
            meta: task.spec && Array.isArray(task.spec.acceptance) ? `${task.spec.acceptance.length} acceptance criteria` : ''
        });
        renderTaskArtifact('01-blueprint.yaml', 'Strategy', task.blueprint, {
            meta: task.blueprint && Array.isArray(task.blueprint.affected_files) ? `${task.blueprint.affected_files.length} files` : ''
        });
        if (hasContract) {
            renderTaskArtifact('02-contract.yaml', 'Boundaries (summarized)', task.contract, {
                meta: Array.isArray(task.contract.touch) ? `${task.contract.touch.length} touch paths` : ''
            });
        }
        renderTaskArtifact('04-implementation-log.yaml', 'Changes', task.implementation_log, {
            meta: task.implementation_log && Array.isArray(task.implementation_log.entries) ? `${task.implementation_log.entries.length} entries` : ''
        });
        renderTaskArtifact('05-validation.json', 'Evidence', task.validation, {
            status: valResult || '',
            meta: task.validation && Array.isArray(task.validation.commands) ? `${task.validation.commands.length} commands` : '',
            open: !!valResult && valResult !== 'passed'
        });
        renderTaskArtifact('06-review.json', 'Verdict', task.review, {
            status: verdict || '',
            open: !!verdict && verdict !== 'approve'
        });
        if (hasTrace) {
            renderTaskArtifact('07-trace.json', 'History (summarized)', task.trace, {
                meta: `${task.trace.attempts_count} attempts`
            });
        }
        renderTaskArtifact('feedback.json', 'Pending Findings', task.feedback, {
            status: task.feedback ? 'pending' : '',
            open: true
        });
    }
});
