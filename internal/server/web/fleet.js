document.addEventListener('DOMContentLoaded', () => {
    const statusList = document.getElementById('status-list');
    const inflightList = document.getElementById('inflight-list');
    const blockedList = document.getElementById('blocked-list');
    const dispatchesList = document.getElementById('dispatches-list');
    const toggleTarget = document.getElementById('toggle-target');
    const toggleReason = document.getElementById('toggle-reason');
    const toggleDisableBtn = document.getElementById('toggle-disable-btn');
    const toggleError = document.getElementById('toggle-error');
    const fleetTokenInput = document.getElementById('fleet-token-input');
    const fleetTokenSaveBtn = document.getElementById('fleet-token-save-btn');
    const pollMs = window.QUORUM_FLEET_POLL_MS || 5000;

    // Manual token entry: lets a non-loopback operator paste the token the
    // server logs out-of-band (never embedded in the page) into
    // localStorage, since getFleetToken() below already reads it from there.
    fleetTokenInput.value = localStorage.getItem('quorumFleetToken') || '';
    fleetTokenSaveBtn.addEventListener('click', () => {
        localStorage.setItem('quorumFleetToken', fleetTokenInput.value.trim());
    });

    // getFleetToken reads the X-Quorum-Fleet-Token to send with a toggle
    // request: the value injected into the page (loopback binds only) or,
    // for a non-loopback bind (where the token is never embedded and is
    // instead logged out-of-band by the server), a value the operator has
    // pasted into localStorage manually.
    const getFleetToken = () => window.QUORUM_FLEET_TOKEN || localStorage.getItem('quorumFleetToken') || '';

    const postToggle = (body) => fetch('/api/fleet/toggle', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Quorum-Fleet-Token': getFleetToken() },
        body: JSON.stringify(body),
    });

    const setToggleError = (text) => {
        toggleError.textContent = text || '';
    };

    const makeEl = (tag, className, text) => {
        const element = document.createElement(tag);
        if (className) element.className = className;
        if (text !== undefined) element.textContent = text;
        return element;
    };

    const clearInto = (container) => {
        container.innerHTML = '';
        return container;
    };

    const renderStatus = (report) => {
        const container = clearInto(statusList);
        const disabled = (report && report.disabled) || [];
        if (disabled.length === 0) {
            container.appendChild(makeEl('p', 'empty', 'All agents/models enabled (no disabled targets).'));
            return;
        }
        const table = makeEl('table');
        const head = makeEl('tr');
        ['Target', 'Status', 'Reason', 'By', 'Age (s)', 'Actions'].forEach((label) => {
            head.appendChild(makeEl('th', null, label));
        });
        table.appendChild(head);
        disabled.forEach((entry) => {
            const row = makeEl('tr');
            row.appendChild(makeEl('td', null, entry.target));
            row.appendChild(makeEl('td', 'status-red', 'disabled'));
            row.appendChild(makeEl('td', null, entry.reason || ''));
            row.appendChild(makeEl('td', null, entry.by || ''));
            row.appendChild(makeEl('td', null, Math.round(entry.age_seconds || 0)));
            const actionsCell = makeEl('td');
            const enableBtn = makeEl('button', null, 'Enable');
            enableBtn.type = 'button';
            enableBtn.addEventListener('click', () => {
                postToggle({ target: entry.target, action: 'enable' })
                    .then((res) => {
                        if (!res.ok) {
                            return res.text().then((text) => { throw new Error(text); });
                        }
                        return res.json();
                    })
                    .then((json) => {
                        setToggleError('');
                        renderStatus(json);
                    })
                    .catch((err) => setToggleError(err.message || String(err)));
            });
            actionsCell.appendChild(enableBtn);
            row.appendChild(actionsCell);
            table.appendChild(row);
        });
        container.appendChild(table);
    };

    const renderInFlight = (items) => {
        const container = clearInto(inflightList);
        if (!items || items.length === 0) {
            container.appendChild(makeEl('p', 'empty', 'No in-flight dispatches.'));
            return;
        }
        const table = makeEl('table');
        const head = makeEl('tr');
        ['Task', 'Dispatch ID', 'Started At', 'Age (s)'].forEach((label) => {
            head.appendChild(makeEl('th', null, label));
        });
        table.appendChild(head);
        items.forEach((item) => {
            const row = makeEl('tr');
            row.appendChild(makeEl('td', null, item.task_id));
            row.appendChild(makeEl('td', null, item.dispatch_id || ''));
            row.appendChild(makeEl('td', null, item.started_at || ''));
            row.appendChild(makeEl('td', null, Math.round(item.age_seconds || 0)));
            table.appendChild(row);
        });
        container.appendChild(table);
    };

    const renderBlocked = (items) => {
        const container = clearInto(blockedList);
        if (!items || items.length === 0) {
            container.appendChild(makeEl('p', 'empty', 'No blocked tasks.'));
            return;
        }
        const table = makeEl('table');
        const head = makeEl('tr');
        ['Task', 'Path', 'Reason', 'Severity', 'Age (s)'].forEach((label) => {
            head.appendChild(makeEl('th', null, label));
        });
        table.appendChild(head);
        items.forEach((item) => {
            const row = makeEl('tr');
            row.appendChild(makeEl('td', null, item.task_id));
            row.appendChild(makeEl('td', null, item.path || ''));
            row.appendChild(makeEl('td', null, item.reason || ''));
            row.appendChild(makeEl('td', null, item.severity || ''));
            row.appendChild(makeEl('td', null, Math.round(item.age_seconds || 0)));
            table.appendChild(row);
        });
        container.appendChild(table);
    };

    const renderDispatches = (items) => {
        const container = clearInto(dispatchesList);
        if (!items || items.length === 0) {
            container.appendChild(makeEl('p', 'empty', 'No recent dispatches.'));
            return;
        }
        const table = makeEl('table');
        const head = makeEl('tr');
        ['Task', 'Phase', 'Agent', 'Model', 'Outcome', 'Result', 'Duration (s)', 'Tokens in/out', 'Cost (USD)', 'At'].forEach((label) => {
            head.appendChild(makeEl('th', null, label));
        });
        table.appendChild(head);
        items.forEach((item) => {
            const row = makeEl('tr');
            row.appendChild(makeEl('td', null, item.task_id));
            row.appendChild(makeEl('td', null, item.phase || ''));
            row.appendChild(makeEl('td', null, item.agent || ''));
            row.appendChild(makeEl('td', null, item.model || ''));
            row.appendChild(makeEl('td', null, item.outcome_class || ''));
            row.appendChild(makeEl('td', null, item.result || ''));
            row.appendChild(makeEl('td', null, item.duration_s != null ? item.duration_s.toFixed(1) : ''));
            const tokens = (item.tokens_in != null || item.tokens_out != null)
                ? `${item.tokens_in != null ? item.tokens_in : '-'} / ${item.tokens_out != null ? item.tokens_out : '-'}`
                : '';
            row.appendChild(makeEl('td', null, tokens));
            row.appendChild(makeEl('td', null, item.cost_usd != null ? item.cost_usd : ''));
            row.appendChild(makeEl('td', null, item.ts || ''));
            table.appendChild(row);
        });
        container.appendChild(table);
    };

    const loadFleetData = () => {
        fetch('/api/fleet/status')
            .then((res) => res.json())
            .then(renderStatus)
            .catch((err) => console.error('Error fetching fleet status:', err));

        fetch('/api/fleet/dispatches')
            .then((res) => res.json())
            .then((data) => {
                renderInFlight(data.in_flight);
                renderBlocked(data.blocked);
                renderDispatches(data.dispatches);
            })
            .catch((err) => console.error('Error fetching fleet dispatches:', err));
    };

    toggleDisableBtn.addEventListener('click', () => {
        const target = toggleTarget.value;
        const reason = toggleReason.value;
        postToggle({ target, action: 'disable', reason })
            .then((res) => {
                if (!res.ok) {
                    return res.text().then((text) => { throw new Error(text); });
                }
                return res.json();
            })
            .then((json) => {
                setToggleError('');
                toggleTarget.value = '';
                toggleReason.value = '';
                renderStatus(json);
            })
            .catch((err) => setToggleError(err.message || String(err)));
    });

    loadFleetData();
    setInterval(loadFleetData, pollMs);
});
