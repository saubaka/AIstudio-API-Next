# AIstudio API Next

[中文文档](README.md) · [Changelog](CHANGELOG.md) · [MIT License](LICENSE)

An independently maintained derivative of [Mag1cFall/AIStudio2API](https://github.com/Mag1cFall/AIStudio2API), providing separate Playground and App Build API channels, tool calling, local-browser sign-in, file uploads, and a Vue console.

This repository keeps the upstream copyright and MIT license. Modifications are maintained by saubaka and this repository's contributors under MIT; third-party licenses remain applicable. See the [Chinese README](README.md#来源版权与许可) for attribution and scope.

## Stack and build

Go 1.25+; Vue 3, TypeScript, Vite, Tailwind CSS; npm lockfile; optional Camoufox or pure Go WAA runtime. Node.js supports 22.13+ within 22.x, or 24+. Python 3.9+ is needed for publication tooling and the macOS background-service helper.

```bash
git clone https://github.com/saubaka/AIstudio-API-Next.git
cd AIstudio-API-Next
cp .env.example .env
# Configure your own PROXY_API_KEY, proxy if needed, and WAA_BACKEND=go.
cd web
npm ci
npm run build
cd ..
go build -mod=readonly -buildvcs=false -trimpath -o aistudio2api ./cmd/aistudio2api
./aistudio2api -open-ui=false
```

On macOS, `python3 local-service.py start --no-open` runs the management process in the background; `stop` stops it. On Windows, copy `.env.example` to `.env` and use `start.bat`, which builds from source when the executable is absent. Full Windows, Linux and macOS instructions are in the [Chinese deployment guide](README.md#部署准备).

Open <http://127.0.0.1:2048>, import an account through the Google sign-in dialog, and then start the generation service. Chromium browsers support a dedicated sign-in window; Safari/manual sign-in accepts a full cURL request or exported session. Headless servers use manual import and may access the console through SSH forwarding.

## API endpoints

| Protocol | Playground | App Build |
| --- | --- | --- |
| OpenAI / Responses | `http://127.0.0.1:2048/playground/v1` | `http://127.0.0.1:2048/build/v1` |
| Anthropic / Gemini | `http://127.0.0.1:2048/playground` | `http://127.0.0.1:2048/build` |

Use your `PROXY_API_KEY` and a model from the selected channel's actual catalog. Fixed channel requests do not silently switch channels. Build does not offer all Playground-specific endpoints; see [channel details](渠道重构说明.md) and [Codex/tool-calling instructions](Codex接入与媒体上传说明.md).

Account eligibility, quotas and cache hits remain controlled by the upstream service. This project is not affiliated with Google.

## Versioning

This repository contains source, tests, dependency manifests, lockfiles, safe configuration examples and documentation. Installed dependencies, binaries, secrets and user data are excluded.

`VERSION` is authoritative. Commit subjects contain only `vX.Y.Z`; bodies contain Simplified Chinese bullet points describing changes and verified results. GitHub Actions checks and builds source; it does not create tags or Releases automatically. Preserve `.env`, `auth/` and `runtime/` when updating your own installation.
