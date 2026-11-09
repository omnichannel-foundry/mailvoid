# Documentation

MailVoid ships two independent [Hugo Book](https://github.com/alex-shpak/hugo-book)
projects:

| Directory | Language | Base URL |
|-----------|----------|----------|
| `docs/en` | English | `/en/` |
| `docs/es` | Español | `/es/` |

Each project is self-contained: it has its own `hugo.toml`, `content/`,
`static/`, `layouts/`, and a vendored copy of the Hugo Book theme.

## Build

```bash
make docs        # build both sites
make docs-en     # build docs/en
make docs-es     # build docs/es
```

Output goes to `docs/en/public` and `docs/es/public`.

## Serve locally

```bash
make docs-serve-en   # http://localhost:1313/en/
make docs-serve-es   # http://localhost:1313/es/
```

The language switcher links to `/en/` and `/es/`, so both sites are expected to
be deployed under the same domain.
