# 001 — MCP Server Facade

> **Priority:** Critical
> **Phase:** 6.1
> **Effort:** Large (3–4 weeks)
> **Status:** Planned

---

## Problem

The Model Context Protocol (MCP) has become the dominant standard for AI tool discovery and invocation. Claude, Cursor, VS Code Copilot, and every major AI coding assistant expect to discover tools via MCP. Our registry stores rich agent and tool configuration but speaks only REST/JSON — making it invisible to the MCP ecosystem.

As of February 2026, the official MCP Registry (backed by Anthropic, Google, Microsoft, OpenAI under the Linux Foundation) catalogs 17,000+ servers. MCPJungle (849 GitHub stars) and Kong's MCP Registry both provide native MCP transport. We cannot participate in this ecosystem without speaking the protocol.

## Solution

Expose the Agentic Registry as a native MCP server so that MCP-aware clients can discover and consume agent configurations directly.

### Deliverables

1. **MCP Transport Layer**
   - Implement Streamable HTTP transport (the current recommended MCP transport)
   - Serve on a dedicated path: `GET /mcp/v1` (SSE) or via stdio for local mode
   - Support the MCP initialization handshake (`initialize` → `initialized`)

2. **Tool Exposure**
   - Expose registry read operations as MCP tools:
     - `list_agents` — returns active agent definitions
     - `get_agent` — returns a single agent with its active prompt
     - `get_discovery` — returns the full composite discovery payload
     - `list_mcp_servers` — returns MCP server configurations (without credentials)
     - `get_model_config` — returns merged model configuration for a scope
   - Each tool includes proper JSON Schema `inputSchema` per MCP spec

3. **Resource Exposure**
   - Expose configuration artifacts as MCP resources:
     - `agent://{agentId}` — agent definition
     - `prompt://{agentId}/active` — active prompt for an agent
     - `config://model` — global model configuration
     - `config://context` — context assembly configuration
   - Support `resources/list` and `resources/read` MCP methods

4. **`mcp.json` Manifest**
   - Publish `/mcp.json` at the server root for automated discovery
   - Include server name, version, supported transports, and tool list

5. **Authentication Bridge**
   - MCP clients authenticate via API key passed as a Bearer token in the MCP transport header
   - Map API key scopes to MCP tool visibility (read-only keys see only read tools)

### Technical Considerations

- Use the official Go MCP SDK (`github.com/mark3labs/mcp-go`) or implement the protocol directly (it's JSON-RPC 2.0 over SSE/HTTP)
- MCP transport runs alongside the existing REST API on the same port; route by path prefix
- Tool results must conform to MCP `content` format (text or embedded resource)
- Consider supporting the `prompts/list` and `prompts/get` MCP methods to expose our prompt templates natively

### Acceptance Criteria

- [ ] Claude Desktop can add our registry as an MCP server and list available tools
- [ ] Cursor can discover and call `list_agents` via MCP
- [ ] `mcp.json` is served at the root and passes validation
- [ ] API key authentication works through MCP transport
- [ ] Existing REST API is unaffected

### Competitive Context

| Competitor | MCP Support |
|---|---|
| MCPJungle | Full (stdio + HTTP) |
| Kong MCP Registry | Full (SSE, commercial) |
| Official MCP Registry | REST API only (discovery, not tool serving) |
| **Us (current)** | **None** |
| **Us (after this item)** | **Full (Streamable HTTP + stdio)** |

### Dependencies

- None (can be built independently of other roadmap items)

### References

- [MCP Specification](https://modelcontextprotocol.io/specification)
- [MCP Go SDK](https://github.com/mark3labs/mcp-go)
- [Official MCP Registry](https://registry.modelcontextprotocol.io/)
