# Contributing to YTEAM

## Portability rule

Do not commit machine-specific paths, usernames, home directories, checkout
locations, or developer workstation names. Use:

- `os.Getwd()` for the current project directory;
- `os.UserHomeDir()` only through the configuration resolver;
- `APPDATA`, `XDG_CONFIG_HOME`, or `YTEAM_HOME` for application data;
- `t.TempDir()` in tests;
- relative paths in documentation and examples.

Examples must work for a fresh checkout on Windows, Linux, and macOS. Never
write a literal path from a developer's laptop into source, tests, fixtures,
documentation, or generated configuration.
