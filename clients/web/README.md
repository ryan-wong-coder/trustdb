# TrustDB Admin Web

Vue 3 + Vite + Pinia + Tailwind 运维控制台，风格对齐 `clients/desktop/frontend`。

## 开发

```bash
npm ci
npm run dev
```

默认将 `/admin/api` 代理到 `http://127.0.0.1:8080`。请先启用服务端 `admin` 配置并运行 `trustdb serve`。

## 生产构建

```bash
npm ci
npm run build
```

将 `admin.web_dir` 指向本目录下的 `dist`（需包含 `index.html`）。
