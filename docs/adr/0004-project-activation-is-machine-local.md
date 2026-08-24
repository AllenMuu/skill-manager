# Project activation is machine-local

Managed project skills use absolute soft links to the local skill library and are not committed to Git. The CLI diagnoses and reconciles links on each machine, avoiding portable-looking but invalid links to another developer's home directory.

## Considered Options

- Commit project links or an activation manifest
- Treat links as local environment state and recreate them per machine
