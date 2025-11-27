// AgentMesh Visualization UI

class VizApp {
    constructor() {
        this.ws = null;
        this.currentRunID = null;
        this.runs = new Map();
        this.events = [];
        this.autoScrollEvents = true;
        
        this.init();
    }
    
    init() {
        this.setupEventListeners();
        this.connectWebSocket();
        this.loadRuns();
        this.loadAndPopulateGraphs();
        this.startPolling();
    }
    
    async loadAndPopulateGraphs() {
        const graphs = await this.loadGraphs();
        const select = document.getElementById('graph-select');
        
        // Clear existing options except the first one
        select.innerHTML = '<option value="">Select a graph...</option>';
        
        // Add graph options
        graphs.forEach(graphID => {
            const option = document.createElement('option');
            option.value = graphID;
            option.textContent = graphID;
            select.appendChild(option);
        });
        
        if (graphs.length === 0) {
            this.showToast('No graphs registered. Register a graph to start runs.', 'warning');
        }
    }
    
    // WebSocket Connection
    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        this.updateConnectionStatus('connecting');
        
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.updateConnectionStatus('connected');
            this.showToast('Connected to server', 'success');
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.updateConnectionStatus('disconnected');
            this.showToast('Disconnected from server', 'error');
            
            // Attempt reconnection after 3 seconds
            setTimeout(() => this.connectWebSocket(), 3000);
        };
        
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
        
        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.handleWebSocketMessage(data);
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };
    }
    
    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'event':
                this.handleNewEvent(data.event || data.data);
                break;
            case 'run_status':
                // data.data.run contains the run object
                const run = data.data?.run || data.run || data.data;
                if (run) {
                    this.handleRunStatus(run);
                }
                break;
            case 'error':
                this.showToast(data.message || 'An error occurred', 'error');
                break;
        }
    }
    
    subscribeToRun(runID) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({
                type: 'subscribe',
                run_id: runID
            }));
        }
    }
    
    unsubscribeFromRun(runID) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({
                type: 'unsubscribe',
                run_id: runID
            }));
        }
    }
    
    updateConnectionStatus(status) {
        const statusEl = document.getElementById('ws-status');
        const indicator = statusEl.querySelector('.status-indicator');
        const text = statusEl.querySelector('.status-text');
        
        indicator.className = `status-indicator ${status}`;
        
        const statusText = {
            connected: 'Connected',
            disconnected: 'Disconnected',
            connecting: 'Connecting...'
        };
        
        text.textContent = statusText[status] || status;
    }
    
    // API Calls
    async fetchAPI(endpoint, options = {}) {
        try {
            const response = await fetch(endpoint, {
                ...options,
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers
                }
            });
            
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            return await response.json();
        } catch (error) {
            console.error('API Error:', error);
            this.showToast(`API Error: ${error.message}`, 'error');
            throw error;
        }
    }
    
    async loadRuns() {
        try {
            const data = await this.fetchAPI('/api/runs');
            const runs = data.runs || [];
            this.renderRunsList(runs);
        } catch (error) {
            console.error('Failed to load runs:', error);
        }
    }
    
    async loadRunDetails(runID) {
        try {
            const [run, eventsData] = await Promise.all([
                this.fetchAPI(`/api/runs/${runID}`),
                this.fetchAPI(`/api/runs/${runID}/events`)
            ]);
            
            this.currentRunID = runID;
            // Handle both array and {events: []} format
            this.events = Array.isArray(eventsData) ? eventsData : (eventsData.events || []);
            
            this.renderRunDetails(run);
            this.renderEvents(this.events);
            this.loadAnalytics(runID);
            this.loadState(runID);
            this.loadGraphVisualization(run.graph_id);
            
            this.subscribeToRun(runID);
        } catch (error) {
            console.error('Failed to load run details:', error);
            this.showToast('Failed to load run details', 'error');
        }
    }
    
    async loadAnalytics(runID) {
        try {
            const analytics = await this.fetchAPI(`/api/runs/${runID}/analytics`);
            this.renderAnalytics(analytics);
        } catch (error) {
            // Analytics may not be available yet for running/recent runs
            if (error.message.includes('404')) {
                // Show "not ready yet" message instead of error
                const costEl = document.getElementById('cost-breakdown');
                const perfEl = document.getElementById('node-performance');
                const bottleneckEl = document.getElementById('bottlenecks');
                
                const notReadyMsg = '<div class="empty-state-sm">Analytics not available yet</div>';
                if (costEl) costEl.innerHTML = notReadyMsg;
                if (perfEl) perfEl.innerHTML = notReadyMsg;
                if (bottleneckEl) bottleneckEl.innerHTML = notReadyMsg;
            } else {
                console.error('Failed to load analytics:', error);
            }
        }
    }
    
    async loadState(runID) {
        try {
            // Load checkpoints list
            const checkpoints = await this.fetchAPI(`/api/runs/${runID}/checkpoints`);
            this.renderStateCheckpoints(checkpoints);
            
            // Load current state
            await this.loadStateContent(runID, 'current');
        } catch (error) {
            console.error('Failed to load state:', error);
            const stateContent = document.getElementById('state-content');
            if (stateContent) {
                stateContent.textContent = 'Failed to load state data';
            }
        }
    }
    
    async loadStateContent(runID, checkpointID) {
        const stateContent = document.getElementById('state-content');
        
        try {
            let state = null;
            
            if (checkpointID === 'current') {
                // Try to get current state first
                try {
                    state = await this.fetchAPI(`/api/runs/${runID}/state`);
                } catch (stateError) {
                    // If state endpoint fails (404), try to get latest checkpoint instead
                    if (stateError.message.includes('404')) {
                        try {
                            const checkpoints = await this.fetchAPI(`/api/runs/${runID}/checkpoints`);
                            if (checkpoints && checkpoints.length > 0) {
                                // Get the latest checkpoint
                                const latest = checkpoints[checkpoints.length - 1];
                                state = await this.fetchAPI(`/api/runs/${runID}/checkpoint/${latest.superstep}`);
                            } else {
                                stateContent.textContent = 'No checkpoints available for this run';
                                return;
                            }
                        } catch (checkpointError) {
                            throw stateError; // Throw original error
                        }
                    } else {
                        throw stateError;
                    }
                }
            } else {
                // Load specific checkpoint
                state = await this.fetchAPI(`/api/runs/${runID}/checkpoint/${checkpointID}`);
            }
            
            if (state && Object.keys(state).length > 0) {
                stateContent.textContent = JSON.stringify(state, null, 2);
            } else {
                stateContent.textContent = 'No state data available';
            }
        } catch (error) {
            // Don't log 404s - they're expected for runs without checkpoints
            if (!error.message.includes('404')) {
                console.error('Failed to load state content:', error);
            }
            if (stateContent) {
                stateContent.textContent = error.message.includes('404') 
                    ? 'No state available for this run'
                    : `Error loading state: ${error.message}`;
            }
        }
    }
    
    async loadTests() {
        try {
            const data = await this.fetchAPI('/api/tests');
            const tests = data.suites || [];
            this.renderTests(tests);
        } catch (error) {
            console.error('Failed to load tests:', error);
        }
    }
    
    async loadGraphVisualization(graphID) {
        if (!graphID) {
            document.getElementById('graph-container').innerHTML = `
                <div class="graph-placeholder">
                    <p>No graph ID available</p>
                    <p class="text-muted">Graph visualization requires a graph ID</p>
                </div>
            `;
            return;
        }
        
        try {
            const data = await this.fetchAPI(`/api/graphs/${graphID}/mermaid`);
            const mermaidCode = data.mermaid || '';
            
            const container = document.getElementById('graph-container');
            
            // Check if mermaid is loaded
            if (typeof mermaid === 'undefined') {
                container.innerHTML = `
                    <div class="graph-placeholder">
                        <p>⚠️ Mermaid library not loaded</p>
                        <p class="text-muted">Check browser console for errors</p>
                        <details>
                            <summary>View raw diagram code</summary>
                            <pre class="mermaid-code">${this.escapeHtml(mermaidCode)}</pre>
                        </details>
                    </div>
                `;
                return;
            }
            
            // Create a unique ID for this mermaid diagram
            const diagramId = `mermaid-${Date.now()}`;
            
            // Render mermaid diagram
            try {
                // Use mermaid.render() for better control
                const { svg } = await mermaid.render(diagramId, mermaidCode);
                container.innerHTML = svg;
                console.log('Mermaid diagram rendered successfully');
            } catch (mermaidError) {
                console.error('Mermaid rendering error:', mermaidError);
                // Fallback: try with plain mermaid div
                try {
                    container.innerHTML = `<div class="mermaid">${mermaidCode}</div>`;
                    await mermaid.run({ nodes: document.querySelectorAll('#graph-container .mermaid') });
                    console.log('Mermaid rendered with fallback method');
                } catch (fallbackError) {
                    console.error('Mermaid fallback also failed:', fallbackError);
                    container.innerHTML = `
                        <div class="graph-placeholder">
                            <p>❌ Mermaid rendering failed</p>
                            <p class="text-muted error">${mermaidError.message || 'Unknown error'}</p>
                            <details>
                                <summary>View raw diagram code</summary>
                                <pre class="mermaid-code">${this.escapeHtml(mermaidCode)}</pre>
                            </details>
                        </div>
                    `;
                }
            }
        } catch (error) {
            console.error('Failed to load graph visualization:', error);
            document.getElementById('graph-container').innerHTML = `
                <div class="graph-placeholder">
                    <p>Failed to load graph</p>
                    <p class="text-muted">${error.message}</p>
                </div>
            `;
        }
    }
    
    async loadGraphs() {
        try {
            const response = await this.fetchAPI('/api/graphs');
            return response.graphs || [];
        } catch (error) {
            console.error('Failed to load graphs:', error);
            this.showToast('Failed to load available graphs', 'error');
            return [];
        }
    }
    
    async executeGraph(graphID, input) {
        try {
            this.showToast('Starting graph execution...', 'info');
            
            const response = await this.fetchAPI(`/api/graphs/${graphID}/run`, {
                method: 'POST',
                body: JSON.stringify(input)
            });
            
            if (response.run_id) {
                this.showToast(`Run started: ${this.truncateID(response.run_id)}`, 'success');
                
                // Reload runs list and select the new run
                await this.loadRuns();
                setTimeout(() => {
                    this.selectRun(response.run_id);
                }, 500);
                
                return response.run_id;
            }
        } catch (error) {
            console.error('Failed to execute graph:', error);
            this.showToast('Failed to start graph execution', 'error');
            throw error;
        }
    }
    
    // Rendering
    renderRunsList(runs) {
        const container = document.getElementById('runs-list');
        
        // Ensure runs is an array
        if (!Array.isArray(runs) || runs.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No runs found</p>
                    <p class="text-muted">Start a graph execution to see runs</p>
                </div>
            `;
            return;
        }
        
        container.innerHTML = runs.map(run => `
            <div class="run-item ${run.id === this.currentRunID ? 'active' : ''}" 
                 data-run-id="${run.id}" 
                 onclick="app.selectRun('${run.id}')">
                <div class="run-item-header">
                    <div class="run-item-id">${this.truncateID(run.id)}</div>
                    <span class="badge ${run.status}">${run.status}</span>
                </div>
                <div class="run-item-meta">
                    ${this.formatTime(run.start_time)}
                </div>
            </div>
        `).join('');
    }
    
    renderRunDetails(run) {
        document.getElementById('no-run-selected').classList.add('hidden');
        document.getElementById('run-viewer').classList.remove('hidden');
        
        document.getElementById('detail-run-id').textContent = this.truncateID(run.id);
        document.getElementById('detail-graph-id').textContent = run.graph_id || 'N/A';
        
        const statusBadge = document.getElementById('detail-status');
        statusBadge.innerHTML = `<span class="badge ${run.status}">${run.status}</span>`;
        
        document.getElementById('detail-start-time').textContent = this.formatTime(run.start_time);
        
        // Calculate duration if we have start and end times
        let duration = 0;
        if (run.start_time && run.end_time) {
            const start = new Date(run.start_time);
            const end = new Date(run.end_time);
            duration = (end - start) / 1000;
        }
        document.getElementById('detail-duration').textContent = this.formatDuration(duration);
        document.getElementById('detail-event-count').textContent = run.events ? run.events.length : 0;
        
        // Update controls based on status
        this.updateControlButtons(run.status);
    }
    
    renderEvents(events) {
        const container = document.getElementById('events-timeline');
        
        if (!events || events.length === 0) {
            container.innerHTML = '<div class="empty-state">No events yet</div>';
            return;
        }
        
        const filterText = document.getElementById('filter-events').value.toLowerCase();
        const filterType = document.getElementById('event-type-filter').value;
        
        const filteredEvents = events.filter(event => {
            const matchesText = !filterText || 
                event.type.toLowerCase().includes(filterText) ||
                (event.node && event.node.toLowerCase().includes(filterText));
            const matchesType = !filterType || event.type === filterType;
            return matchesText && matchesType;
        });
        
        container.innerHTML = filteredEvents.map(event => `
            <div class="event-card ${event.type}" data-event-type="${event.type}">
                <div class="event-icon">${this.getEventIcon(event.type)}</div>
                <div class="event-content">
                    <div class="event-header">
                        <span class="event-type">${this.formatEventType(event.type)}</span>
                        <span class="event-time">${this.formatTime(event.timestamp)}</span>
                    </div>
                    ${event.node ? `<div class="event-node">Node: ${event.node}</div>` : ''}
                    ${this.renderEventDetails(event)}
                </div>
            </div>
        `).join('');
        
        if (this.autoScrollEvents) {
            container.scrollTop = container.scrollHeight;
        }
    }
    
    renderEventDetails(event) {
        const details = [];
        
        if (event.payload) {
            if (event.payload.model_name) {
                details.push(`Model: ${event.payload.model_name}`);
            }
            if (event.payload.est_cost_usd) {
                details.push(`Cost: $${event.payload.est_cost_usd.toFixed(4)}`);
            }
            if (event.payload.total_tokens) {
                details.push(`Tokens: ${event.payload.total_tokens}`);
            }
            if (event.payload.error) {
                details.push(`Error: ${event.payload.error}`);
            }
        }
        
        if (event.duration) {
            details.push(`Duration: ${this.formatDuration(event.duration)}`);
        }
        
        return details.length > 0 
            ? `<div class="event-details">${details.join(' • ')}</div>`
            : '';
    }
    
    renderAnalytics(analytics) {
        // Update quick stats
        document.getElementById('stat-cost').textContent = 
            `$${(analytics.total_cost || 0).toFixed(2)}`;
        document.getElementById('stat-tokens').textContent = 
            analytics.total_tokens || 0;
        document.getElementById('stat-nodes').textContent = 
            Object.keys(analytics.node_metrics || {}).length;
        document.getElementById('stat-errors').textContent = 
            analytics.error_count || 0;
        
        // Cost breakdown
        this.renderCostBreakdown(analytics);
        
        // Node performance
        this.renderNodePerformance(analytics);
        
        // Bottlenecks
        this.renderBottlenecks(analytics);
    }
    
    renderCostBreakdown(analytics) {
        const container = document.getElementById('cost-breakdown');
        
        if (!analytics.cost_by_model || Object.keys(analytics.cost_by_model).length === 0) {
            container.innerHTML = '<div class="empty-state-sm">No cost data</div>';
            return;
        }
        
        const items = Object.entries(analytics.cost_by_model)
            .sort((a, b) => b[1] - a[1])
            .map(([model, cost]) => `
                <div class="cost-item">
                    <span class="item-label">${model}</span>
                    <span class="item-value">$${cost.toFixed(4)}</span>
                </div>
            `).join('');
        
        container.innerHTML = items;
    }
    
    renderNodePerformance(analytics) {
        const container = document.getElementById('node-performance');
        
        if (!analytics.node_metrics || Object.keys(analytics.node_metrics).length === 0) {
            container.innerHTML = '<div class="empty-state-sm">No performance data</div>';
            return;
        }
        
        const items = Object.entries(analytics.node_metrics)
            .sort((a, b) => b[1].total_duration - a[1].total_duration)
            .slice(0, 5)
            .map(([node, metrics]) => `
                <div class="performance-item">
                    <div>
                        <div class="item-label">${node}</div>
                        <div class="text-muted" style="font-size: 11px;">
                            ${metrics.execution_count} execution(s)
                        </div>
                    </div>
                    <span class="item-value">${this.formatDuration(metrics.avg_duration)}</span>
                </div>
            `).join('');
        
        container.innerHTML = items;
    }
    
    renderBottlenecks(analytics) {
        const container = document.getElementById('bottlenecks');
        
        if (!analytics.bottlenecks || analytics.bottlenecks.length === 0) {
            container.innerHTML = '<div class="empty-state-sm">No bottlenecks detected ✓</div>';
            return;
        }
        
        const items = analytics.bottlenecks.map(bottleneck => `
            <div class="bottleneck-item">
                <div>
                    <div class="item-label">${bottleneck.node_id}</div>
                    <div class="text-muted" style="font-size: 11px;">
                        ${bottleneck.type} • ${bottleneck.impact}
                    </div>
                    <div class="text-muted" style="font-size: 11px; margin-top: 4px;">
                        ${bottleneck.description}
                    </div>
                </div>
            </div>
        `).join('');
        
        container.innerHTML = items;
    }
    
    renderStateCheckpoints(response) {
        const selector = document.getElementById('checkpoint-selector');
        
        selector.innerHTML = '<option value="current">Current State</option>';
        
        // Extract checkpoints array from response object
        const checkpoints = response.checkpoints || [];
        
        if (checkpoints.length > 0) {
            checkpoints.forEach(cp => {
                const option = document.createElement('option');
                option.value = cp.superstep || cp.id;
                option.textContent = `Step ${cp.superstep || cp.id} (${this.formatTime(cp.timestamp)})`;
                selector.appendChild(option);
            });
        }
    }
    
    renderTests(tests) {
        const container = document.getElementById('tests-list');
        
        if (!Array.isArray(tests) || tests.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No tests configured</p>
                    <p class="text-muted">Create test scenarios to validate graph execution</p>
                </div>
            `;
            return;
        }
        
        container.innerHTML = tests.map(test => `
            <div class="test-card">
                <div class="test-card-header">
                    <span class="test-name">${test.name}</span>
                    <span class="test-status ${test.status}">${test.status}</span>
                </div>
                <div class="text-muted" style="margin-bottom: 12px;">
                    ${test.description || 'No description'}
                </div>
                <button class="btn btn-primary btn-sm" onclick="app.runTest('${test.id}')">
                    <span class="icon">▶️</span> Run Test
                </button>
            </div>
        `).join('');
    }
    
    // Event Handlers
    handleNewEvent(event) {
        if (!this.currentRunID || event.run_id !== this.currentRunID) {
            return;
        }
        
        this.events.push(event);
        this.renderEvents(this.events);
    }
    
    handleRunStatus(run) {
        console.log('Run status update:', run.id, run.status);
        
        if (this.currentRunID === run.id) {
            // Update the current run details
            this.renderRunDetails(run);
            
            // If run just completed, reload analytics
            if (run.status === 'completed' || run.status === 'failed') {
                this.loadAnalytics(run.id);
            }
        }
        
        // Update the specific run in the list without full reload
        const runElement = document.querySelector(`[data-run-id="${run.id}"]`);
        if (runElement) {
            const badge = runElement.querySelector('.badge');
            if (badge) {
                badge.className = `badge ${run.status}`;
                badge.textContent = run.status;
            }
        }
    }
    
    selectRun(runID) {
        if (this.currentRunID) {
            this.unsubscribeFromRun(this.currentRunID);
        }
        
        this.loadRunDetails(runID);
        
        // Update active state in list
        document.querySelectorAll('.run-item').forEach(item => {
            item.classList.toggle('active', item.dataset.runId === runID);
        });
    }
    
    updateControlButtons(status) {
        const pauseBtn = document.getElementById('btn-pause');
        const resumeBtn = document.getElementById('btn-resume');
        const stopBtn = document.getElementById('btn-stop');
        
        pauseBtn.disabled = status !== 'running';
        resumeBtn.disabled = status !== 'paused';
        stopBtn.disabled = status === 'completed' || status === 'failed';
    }
    
    async controlRun(action) {
        if (!this.currentRunID) return;
        
        try {
            await this.fetchAPI(`/api/runs/${this.currentRunID}/control`, {
                method: 'POST',
                body: JSON.stringify({ action })
            });
            
            this.showToast(`Run ${action} command sent`, 'success');
        } catch (error) {
            console.error('Control command failed:', error);
        }
    }
    
    async runTest(testID) {
        try {
            await this.fetchAPI(`/api/tests/${testID}/run`, {
                method: 'POST'
            });
            
            this.showToast('Test started', 'success');
            this.loadTests();
        } catch (error) {
            console.error('Failed to run test:', error);
        }
    }
    
    // Setup Event Listeners
    setupEventListeners() {
        // Tab switching
        document.querySelectorAll('.tab-button').forEach(button => {
            button.addEventListener('click', () => {
                const tabName = button.dataset.tab;
                this.switchTab(tabName);
            });
        });
        
        // Start new run
        document.getElementById('start-run-btn').addEventListener('click', async () => {
            const graphID = document.getElementById('graph-select').value;
            const inputText = document.getElementById('run-input').value.trim();
            
            if (!graphID) {
                this.showToast('Please select a graph', 'error');
                return;
            }
            
            // Parse input - try JSON first, fallback to plain text
            let input;
            if (inputText) {
                try {
                    input = JSON.parse(inputText);
                } catch {
                    input = { input: inputText };
                }
            } else {
                input = { input: 'start' };
            }
            
            try {
                await this.executeGraph(graphID, input);
                // Clear the form
                document.getElementById('run-input').value = '';
            } catch (error) {
                // Error already shown in executeGraph
            }
        });
        
        // Refresh runs
        document.getElementById('refresh-runs').addEventListener('click', () => {
            this.loadRuns();
        });
        
        // Event filters
        document.getElementById('filter-events').addEventListener('input', () => {
            this.renderEvents(this.events);
        });
        
        document.getElementById('event-type-filter').addEventListener('change', () => {
            this.renderEvents(this.events);
        });
        
        document.getElementById('auto-scroll-events').addEventListener('change', (e) => {
            this.autoScrollEvents = e.target.checked;
        });
        
        document.getElementById('clear-events').addEventListener('click', () => {
            this.events = [];
            this.renderEvents([]);
        });
        
        // Control buttons
        document.getElementById('btn-pause').addEventListener('click', () => {
            this.controlRun('pause');
        });
        
        document.getElementById('btn-resume').addEventListener('click', () => {
            this.controlRun('resume');
        });
        
        document.getElementById('btn-stop').addEventListener('click', () => {
            this.controlRun('stop');
        });
        
        document.getElementById('btn-restart').addEventListener('click', () => {
            this.controlRun('restart');
        });
        
        // State controls
        document.getElementById('checkpoint-selector').addEventListener('change', (e) => {
            if (this.currentRunID) {
                this.loadStateContent(this.currentRunID, e.target.value);
            }
        });
        
        document.getElementById('state-refresh').addEventListener('click', () => {
            this.loadState(this.currentRunID);
        });
        
        // Test controls
        document.getElementById('run-tests').addEventListener('click', () => {
            this.loadTests();
        });
    }
    
    switchTab(tabName) {
        // Update buttons
        document.querySelectorAll('.tab-button').forEach(button => {
            button.classList.toggle('active', button.dataset.tab === tabName);
        });
        
        // Update panes
        document.querySelectorAll('.tab-pane').forEach(pane => {
            pane.classList.toggle('active', pane.id === `tab-${tabName}`);
        });
        
        // Load data for specific tabs
        if (tabName === 'tests' && this.currentRunID) {
            this.loadTests();
        }
    }
    
    // Utilities
    truncateID(id) {
        if (!id) return 'N/A';
        return id.length > 12 ? id.substring(0, 12) + '...' : id;
    }
    
    formatTime(timestamp) {
        if (!timestamp) return 'N/A';
        const date = new Date(timestamp);
        return date.toLocaleString();
    }
    
    formatDuration(duration) {
        if (!duration) return '0ms';
        
        // Handle both nanoseconds (number) and string durations
        let ms;
        if (typeof duration === 'string') {
            // Parse Go duration string (e.g., "1.5s", "100ms", "1m30s")
            if (duration.includes('m')) {
                const parts = duration.split('m');
                const minutes = parseFloat(parts[0]);
                const seconds = parts[1] ? parseFloat(parts[1].replace('s', '')) : 0;
                ms = (minutes * 60 + seconds) * 1000;
            } else if (duration.includes('s')) {
                ms = parseFloat(duration.replace('s', '')) * 1000;
            } else if (duration.includes('ms')) {
                ms = parseFloat(duration.replace('ms', ''));
            } else {
                ms = parseFloat(duration) / 1000000; // nanoseconds to ms
            }
        } else {
            ms = duration / 1000000; // nanoseconds to ms
        }
        
        if (ms < 1000) {
            return `${ms.toFixed(0)}ms`;
        } else if (ms < 60000) {
            return `${(ms / 1000).toFixed(2)}s`;
        } else {
            const minutes = Math.floor(ms / 60000);
            const seconds = ((ms % 60000) / 1000).toFixed(0);
            return `${minutes}m ${seconds}s`;
        }
    }
    
    getEventIcon(type) {
        const icons = {
            node_start: '🟦',
            node_complete: '✅',
            node_error: '❌',
            step_start: '▶️',
            step_end: '⏹️',
            state_update: '💾',
            checkpoint: '📍',
            interrupt: '⏸️'
        };
        return icons[type] || '📝';
    }
    
    formatEventType(type) {
        return type.split('_').map(word => 
            word.charAt(0).toUpperCase() + word.slice(1)
        ).join(' ');
    }
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            <div class="icon">${this.getToastIcon(type)}</div>
            <div>${message}</div>
        `;
        
        container.appendChild(toast);
        
        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }
    
    getToastIcon(type) {
        const icons = {
            success: '✅',
            error: '❌',
            warning: '⚠️',
            info: 'ℹ️'
        };
        return icons[type] || 'ℹ️';
    }
    
    startPolling() {
        // Poll for run updates every 5 seconds
        setInterval(() => {
            if (!this.currentRunID) {
                this.loadRuns();
            }
        }, 5000);
    }
}

// Initialize app when DOM is ready
let app;
document.addEventListener('DOMContentLoaded', () => {
    app = new VizApp();
});
