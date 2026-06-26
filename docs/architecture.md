# Architecture

[中文](zh-CN/architecture.md)

## Layered Design

Atlas is divided into entry layer, orchestration layer, core loop, capability layer, and persistence layer. All entry points share the same `runtime.Runtime`, and the core agent loop remains pure and side-effect-free.

```mermaid
graph TD
    subgraph Entry Layer
        CLI[CLI]
        ACP[ACP Adapter]
        WX[WeChat Channel]
    end

    subgraph Orchestration Layer
        RT[runtime.RunTurn]
        MW[Memory Worker]
    end

    subgraph Core Loop
        AG[agent loop]
        PR[Provider Interface]
    end

    subgraph Capability Layer
        TOOLS[Tool Registry]
        PRMPT[System Prompt]
        CMP[Context Compaction]
    end

    subgraph Persistence Layer
        TR[transcript]
        SS[session SQLite]
        MS[memory SQLite + FTS5]
    end

    CLI --> RT
    ACP --> RT
    WX --> RT
    RT --> AG
    RT --> MW
    AG --> PR
    AG --> TOOLS
    RT --> PRMPT
    RT --> CMP
    RT --> MS
    AG --> TR
    RT --> SS
```

## Core Loop

A turn starts with user input: appended to the transcript, then the model is called in a loop. When the model returns text deltas, they are streamed out; when it returns tool calls, they are executed in order and results are written back to the transcript. The loop ends when there are no tool calls, an error occurs, or the step limit is reached.

```mermaid
sequenceDiagram
    participant RT as runtime.RunTurn
    participant AG as agent loop
    participant PR as provider.Stream
    participant TR as transcript
    participant TOOLS as tool.Registry

    RT->>AG: user input
    AG->>TR: append user message
    loop step < maxSteps
        AG->>PR: Stream(ChatRequest)
        PR-->>AG: streaming delta
        PR-->>AG: ChatResponse
        AG->>TR: append assistant message
        alt no tool calls
            AG-->>RT: return final reply
        else has tool calls
            loop each tool call in order
                AG->>TOOLS: Run(call)
                TOOLS-->>AG: result or error
                AG->>TR: append tool message
            end
        end
    end
    AG-->>RT: step limit exceeded
```

Key constraints:

- Every tool call has a paired tool result, in the same order the model returned them.
- Tool errors are written back as model-visible tool results, letting the model adjust accordingly.
- The loop ends when there are no tool calls, an error occurs, or `max_steps` (default 20) is reached.

## Long-Term Memory

The memory system works asynchronously via a background worker. There are three trigger conditions: session message count reaches an incremental threshold, the user explicitly asks to remember something, or context compaction completes. When triggered, an extraction task is enqueued. The worker only processes new messages since the last boundary, calls the model to extract memory entries and writes them to the database, then refreshes summaries for affected scopes. Relevant memories are automatically retrieved and injected into the system prompt at the start of the next session.

Memory types:

- `instruction`: long-term user preferences or constraints
- `fact`: project facts
- `workflow`: reusable project workflows

Organized by `global` (cross-project) and `project` (per project directory) scopes.

## Context Compaction and Todos

When context compaction triggers, earlier messages are summarized while recent messages are kept to continue the conversation. If the model has been using `todo_write` to track tasks, the last todo list is extracted from the transcript and incomplete items are injected into the summary prompt. This ensures the model retains awareness of pending work after compaction, without persisting todo state to the database.
