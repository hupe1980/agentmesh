package viz

// OpenAPISpec returns the OpenAPI 3.0 specification for the viz REST API
func OpenAPISpec() string {
	return `openapi: 3.0.3
info:
  title: AgentMesh Visualization API
  description: |
    REST API for the AgentMesh visualization server. Provides endpoints for:
    - Graph management and execution
    - Run monitoring and control
    - Event streaming and analytics
    - Test management
  version: 1.0.0
  contact:
    name: AgentMesh
    url: https://github.com/hupe1980/agentmesh

servers:
  - url: http://localhost:8080
    description: Local development server

tags:
  - name: graphs
    description: Graph registration and execution
  - name: runs
    description: Run monitoring and control
  - name: tests
    description: Test suite management
  - name: websocket
    description: Real-time event streaming

paths:
  /api/graphs:
    get:
      tags: [graphs]
      summary: List all registered graphs
      description: Returns a list of graph IDs that have been registered with the server
      responses:
        '200':
          description: Successful response
          content:
            application/json:
              schema:
                type: object
                properties:
                  graphs:
                    type: array
                    items:
                      type: string
                    example: ["weather-agent", "chat-agent", "workflow-1"]

  /api/graphs/{graphId}/run:
    post:
      tags: [graphs]
      summary: Execute a graph
      description: Starts execution of a registered graph with the provided input
      parameters:
        - name: graphId
          in: path
          required: true
          description: ID of the graph to execute
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: true
              example:
                query: "What's the weather in Berlin?"
      responses:
        '200':
          description: Graph execution started
          content:
            application/json:
              schema:
                type: object
                properties:
                  run_id:
                    type: string
                    description: Unique identifier for this execution
                    example: "3f4d5e6a7b8c9d0e"
        '404':
          description: Graph not found
        '500':
          description: Execution failed

  /api/graphs/{graphId}/mermaid:
    get:
      tags: [graphs]
      summary: Get graph visualization
      description: Returns a Mermaid flowchart diagram of the graph topology
      parameters:
        - name: graphId
          in: path
          required: true
          schema:
            type: string
        - name: direction
          in: query
          required: false
          description: Flow direction (TD, LR, etc.)
          schema:
            type: string
            default: TD
      responses:
        '200':
          description: Mermaid diagram
          content:
            application/json:
              schema:
                type: object
                properties:
                  mermaid:
                    type: string
                    example: "graph TD\\n  start --> node1\\n  node1 --> end"
        '404':
          description: Graph not found

  /api/runs:
    get:
      tags: [runs]
      summary: List all runs
      description: Returns a list of all execution runs with their status
      parameters:
        - name: status
          in: query
          required: false
          description: Filter by run status
          schema:
            type: string
            enum: [running, completed, failed, paused]
        - name: limit
          in: query
          required: false
          description: Maximum number of runs to return
          schema:
            type: integer
            default: 100
      responses:
        '200':
          description: List of runs
          content:
            application/json:
              schema:
                type: object
                properties:
                  runs:
                    type: array
                    items:
                      $ref: '#/components/schemas/RunSummary'

  /api/runs/{runId}:
    get:
      tags: [runs]
      summary: Get run details
      description: Returns detailed information about a specific run
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Run details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/RunDetails'
        '404':
          description: Run not found

  /api/runs/{runId}/events:
    get:
      tags: [runs]
      summary: Get run events
      description: Returns all events for a specific run
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
        - name: type
          in: query
          required: false
          description: Filter by event type
          schema:
            type: string
            enum: [node_start, node_complete, node_error, step_start, step_end, state_update, checkpoint]
        - name: node
          in: query
          required: false
          description: Filter by node name
          schema:
            type: string
        - name: offset
          in: query
          required: false
          description: Number of events to skip
          schema:
            type: integer
            default: 0
      responses:
        '200':
          description: List of events
          content:
            application/json:
              schema:
                type: object
                properties:
                  events:
                    type: array
                    items:
                      $ref: '#/components/schemas/Event'
        '404':
          description: Run not found

  /api/runs/{runId}/state:
    get:
      tags: [runs]
      summary: Get run state
      description: Returns the current state of a run from the latest checkpoint
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
        - name: superstep
          in: query
          required: false
          description: Load state from specific superstep
          schema:
            type: integer
      responses:
        '200':
          description: Run state
          content:
            application/json:
              schema:
                type: object
                additionalProperties: true
        '404':
          description: Run or checkpoint not found

  /api/runs/{runId}/analytics:
    get:
      tags: [runs]
      summary: Get run analytics
      description: Returns cost and performance analytics for a completed run
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Analytics data
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Analytics'
        '404':
          description: Run not found or analytics not available

  /api/runs/{runId}/control:
    post:
      tags: [runs]
      summary: Control run execution
      description: Send control commands to a running execution (pause, resume, stop)
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                action:
                  type: string
                  enum: [pause, resume, stop, restart]
                  description: Control action to perform
                target:
                  type: integer
                  description: Optional target superstep for time travel
              required: [action]
              example:
                action: pause
      responses:
        '200':
          description: Command accepted
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                    example: "paused"
        '400':
          description: Invalid command or run not controllable
        '404':
          description: Run not found
        '501':
          description: Command not implemented

  /api/tests:
    get:
      tags: [tests]
      summary: List test suites
      description: Returns all registered test suites
      responses:
        '200':
          description: List of test suites
          content:
            application/json:
              schema:
                type: object
                properties:
                  suites:
                    type: array
                    items:
                      $ref: '#/components/schemas/TestSuite'

  /api/tests/suite:
    post:
      tags: [tests]
      summary: Create test suite
      description: Creates a new test suite with test cases
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TestSuiteCreate'
      responses:
        '200':
          description: Test suite created
        '400':
          description: Invalid test suite definition

  /api/tests/run:
    post:
      tags: [tests]
      summary: Run tests
      description: Executes tests from a test suite
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                suite_id:
                  type: string
                graph_id:
                  type: string
              required: [suite_id, graph_id]
      responses:
        '200':
          description: Test results
          content:
            application/json:
              schema:
                type: object
                properties:
                  results:
                    type: array
                    items:
                      $ref: '#/components/schemas/TestResult'

  /api/tests/{suiteId}/{testName}:
    delete:
      tags: [tests]
      summary: Delete test
      description: Removes a test case from a suite
      parameters:
        - name: suiteId
          in: path
          required: true
          schema:
            type: string
        - name: testName
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Test deleted
        '404':
          description: Test not found

  /ws:
    get:
      tags: [websocket]
      summary: WebSocket connection
      description: |
        Establishes a WebSocket connection for real-time event streaming.
        
        **Message Types:**
        - event: Execution events (node_start, node_complete, etc.)
        - lifecycle: Run lifecycle events (started, completed, failed)
        - state: State update notifications
        
        **Subscription:**
        Send a message to subscribe to specific runs:
        ` + "```json" + `
        {
          "type": "subscribe",
          "run_id": "abc123"
        }
        ` + "```" + `
      responses:
        '101':
          description: WebSocket connection established
        '400':
          description: Invalid WebSocket request

components:
  schemas:
    RunSummary:
      type: object
      properties:
        id:
          type: string
        graph_id:
          type: string
        status:
          type: string
          enum: [running, completed, failed, paused]
        start_time:
          type: string
          format: date-time
        end_time:
          type: string
          format: date-time
          nullable: true
        duration:
          type: number
          description: Duration in seconds

    RunDetails:
      allOf:
        - $ref: '#/components/schemas/RunSummary'
        - type: object
          properties:
            events:
              type: array
              items:
                $ref: '#/components/schemas/Event'

    Event:
      type: object
      properties:
        id:
          type: string
        run_id:
          type: string
        type:
          type: string
          enum: [node_start, node_complete, node_error, step_start, step_end, state_update, checkpoint, interrupt]
        timestamp:
          type: string
          format: date-time
        node:
          type: string
          nullable: true
        superstep:
          type: integer
        payload:
          type: object
          properties:
            est_cost_usd:
              type: number
            total_tokens:
              type: integer
            model_name:
              type: string
            error:
              type: string

    Analytics:
      type: object
      properties:
        run_id:
          type: string
        graph_id:
          type: string
        total_cost:
          type: number
          description: Total cost in USD
        total_tokens:
          type: integer
        cost_by_model:
          type: object
          additionalProperties:
            type: number
        cost_by_node:
          type: object
          additionalProperties:
            type: number
        node_metrics:
          type: object
          additionalProperties:
            $ref: '#/components/schemas/NodeMetrics'
        bottlenecks:
          type: array
          items:
            $ref: '#/components/schemas/Bottleneck'

    NodeMetrics:
      type: object
      properties:
        execution_count:
          type: integer
        total_duration:
          type: number
          description: Total duration in seconds
        avg_duration:
          type: number
        max_duration:
          type: number
        min_duration:
          type: number

    Bottleneck:
      type: object
      properties:
        node:
          type: string
        type:
          type: string
          enum: [slow, expensive]
        value:
          type: number
        unit:
          type: string

    TestSuite:
      type: object
      properties:
        suite_id:
          type: string
        graph_id:
          type: string
        tests:
          type: array
          items:
            $ref: '#/components/schemas/TestCase'

    TestSuiteCreate:
      type: object
      properties:
        suite_id:
          type: string
        graph_id:
          type: string
        tests:
          type: array
          items:
            type: object
            properties:
              name:
                type: string
              input:
                type: object
                additionalProperties: true
              expected_output:
                type: object
                additionalProperties: true
      required: [suite_id, graph_id, tests]

    TestCase:
      type: object
      properties:
        name:
          type: string
        input:
          type: object
        expected_output:
          type: object
        last_run:
          type: string
          format: date-time
          nullable: true
        status:
          type: string
          enum: [passed, failed, not_run]

    TestResult:
      type: object
      properties:
        test_name:
          type: string
        passed:
          type: boolean
        actual_output:
          type: object
        expected_output:
          type: object
        error:
          type: string
          nullable: true
        duration:
          type: number
          description: Test duration in seconds
`
}
