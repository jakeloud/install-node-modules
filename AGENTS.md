# Lightweight NPM Install Clone

A minimal CLI tool that replicates core npm install functionality: parses `package.json`, resolves dependencies (semver-compatible versions), installs to `node_modules`, and runs postinstall scripts. Built to be ultra-lightweight (<128MB memory) by leveraging system tools (`curl`, `jq`, `tar`) instead of hand-rolling HTTP/JSON/extraction logic.

## Project Goals

- **Minimal Footprint**: Target runtime <50MB memory even for large dependency trees
- **Zero/Low Dependencies**: Use system tools for heavy lifting; avoid crate/pip bloat
- **Docker/Debian Ready**: Works on Debian 11/12 or Docker base images with minimal apt install
- **Core Features First**: package.json parsing, semver resolution, node_modules installation, postinstall scripts

## Key Challenges

1. **Subprocess Management**: Spawning many curl processes can spike memory; need throttling/sequencing
2. **Semver Resolution**: Hand-roll simple semver logic or leverage jq; complex version ranges may be tricky
3. **Circular Dependency Handling**: Detect and gracefully handle cycles in dependency graphs
4. **Error Resilience**: Script-based approaches are fragile; need robust error handling
5. **Tool Availability**: Assume curl/jq/tar exist; provide clear install instructions if missing
6. **Postinstall Script Execution**: Must spawn bash/node correctly with proper environment

## Focus Areas

- **Streaming**: Pipe curl output directly to tar to avoid buffering entire packages in memory
- **Level-by-Level Processing**: Process dependencies in waves rather than building full in-memory graph
- **Parallelism Cap**: Limit concurrent downloads to 4 to control memory usage
- **Minimal Output**: node_modules structure that Node.js can actually require()

## Limitations

- No lockfile support (package-lock.json ignored for MVP)
- No devDependencies handling (install all by default)
- No workspace support
- JSR registry support is future work, not MVP

## Environment Requirements

- Debian 11/12 or Docker with debian:11/debian:12 base
- System tools: curl, tar, bash
- Optional: jq (install via `apt install jq` if needed for complex JSON queries)
- Node.js runtime required for postinstall scripts only

## Success Criteria

- Successfully installs dependencies from a standard package.json with 10-50 packages
- Memory usage stays under 128MB during install
- Postinstall scripts execute correctly
- Works in Docker container with 128MB memory limit
