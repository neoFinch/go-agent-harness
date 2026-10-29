# Agent Harness

A conversational harness that sends requests to a model and manages the conversation context.

## Language

**Session**:
A distinct conversation with its own history. Starting a new session leaves previous sessions' saved conversations on disk.

**Transcript**:
The durable record of a session's conversation. Starting a new session with `/clear` does not delete the preceding session's transcript.

**Project memory**:
Information explicitly requested by the user to be remembered for the project across sessions. Ordinary conversation does not automatically become project memory.

**Context budget**:
The total token allowance for a model request, including conversation history, instructions, the current prompt, and reserved space for the model's answer. A selected 32K context budget covers all of these together.
_Avoid_: Context length (ambiguous between an input allowance and a total allowance)

**Output reserve**:
The portion of the context budget set aside for the next model response, including reasoning tokens where applicable. It is an upper allowance, not a target response length.

**Compaction**:
The replacement of older conversation content with a shorter representation that preserves information needed to continue the conversation.

**Compaction threshold**:
The context usage level at which the harness initiates its compaction policy, leaving room before the context budget is exhausted.
