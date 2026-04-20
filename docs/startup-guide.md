# 启动与联调指南（流媒体 Router）

**目录：** §1 Docker 启动 · §2 环境变量与令牌有效期 · §3 session-api · §4 创建会话 · §5 WHIP · §6 WHEP · §7 MediaMTX 与端口 · §8 停止与会话删除 · §9 常见问题

---

## 1) Docker 启动

```bash
cd avatar-stack
cp .env.example .env
# 按需编辑 .env（见 §2）
docker compose up -d --build
```

**服务与依赖：** `redis` → `session-api`；`session-api` + `coturn` → `mediamtx`（`mediamtx` 依赖 `session-api`，以便 HTTP 鉴权可用）。

**媒体路径：** `avatar/{session_id}/live`（WHIP 推流与 WHEP 拉流同一路径；鉴权走 session-api 签发的短期令牌）。

---

## 2) 环境变量与令牌有效期

通过 **`docker-compose.yml` 中 `session-api.environment`** 或根目录 **`.env`** 注入（compose 会做变量替换）。

| 变量 | 作用 |
|------|------|
| **`MEDIAMTX_WEBRTC_BASE_URL`** | 写入 `POST /sessions` 返回的 WHIP/WHEP URL。**浏览器所在机器必须能访问该地址**（本机开发常用 `http://localhost:8889`；局域网其他设备需改成宿主机 IP 或域名，如 `http://192.168.1.10:8889`）。 |
| **`WHIP_USERNAME` / `WHEP_USERNAME`** | Basic Auth 用户名（默认 `publisher` / `viewer`），须与客户端、FFmpeg 里写的一致。 |
| **`SESSION_TTL`** | 若**设置且非空**，单独决定 **Redis 会话 TTL = 令牌有效期**（覆盖下方 `MEDIAMTX_TOKEN_TTL`）。Go 时长格式，如 `24h`、`30m`。 |
| **`MEDIAMTX_TOKEN_TTL`** | 未设置 `SESSION_TTL` 时使用，默认 **`1h`**。 |
| **`INTERNAL_AUTH_ALLOWED_IPS`** | 可选。非空时，**仅**列表中的 IP（或 `remoteAddr` 串）可访问 `/internal/mediamtx/*`；为空时默认允许**回环 + 私网**（Docker 网桥一般可用）。 |

**令牌说明：** Redis 中存的是 **bcrypt 哈希**；明文在 **`POST /sessions` 响应里只出现一次**。`GET /sessions/{id}` 中 `publish.password` / `playback.password` 为空。

---

## 3) session-api

- 监听：**`8080`**（compose 映射 `http://localhost:8080`）。
- 健康检查：

```bash
curl -s http://localhost:8080/healthz | jq
curl -s http://localhost:8080/readyz | jq
```

- **查询会话（不含密码）：**

```bash
curl -s http://localhost:8080/sessions/<session_id> | jq
```

---

## 4) 创建会话

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar"}' | jq
```

请保存响应中的 **`session_id`**、**`publish.password`**、**`playback.password`**、`publish.whip_url`、`playback.whep_url`。

---

## 5) WHIP（推流）

**顺序：** 先 **`POST /sessions`**，再用返回的 **`publish.password`** 作为 Basic Auth **密码**（用户名为 **`publish.username`**，默认 `publisher`），在同一 `session_id` 上 WHIP 并保持连接。

可用仓库内 **`tools/whip_push_test.html`**（须用 `http://` 打开页面），或 FFmpeg（需支持 `whip` muxer）：

```bash
SESSION_ID='<paste uuid>'
PUB_PASS='<paste publish.password>'
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
  -c:a libopus -ar 48000 -ac 2 -b:a 128k \
  -f whip "http://publisher:${PUB_PASS}@localhost:8889/avatar/${SESSION_ID}/live/whip"
```

若从局域网其他机器推流，将 `localhost` 换成 **`MEDIAMTX_WEBRTC_BASE_URL` 里使用的主机名/IP**，并与 `.env` 一致。

---

## 6) WHEP（播放）

在 WHIP **已成功推流并保持** 的前提下，用 **`playback.whep_url`**，密码为 **`playback.password`**，用户名为 **`playback.username`**（默认 `viewer`）。可用 **`tools/whep_test.html`**。

若 MediaMTX 返回 path 上暂无流（如 **404**），多为尚未 WHIP、WHIP 已断或会话/令牌已过期。

---

## 7) MediaMTX 与端口

当前 `mediamtx.yml` 使用 **`authMethod: http`**，**推流/拉流**鉴权由 **session-api** 完成。

**控制面与指标：** **`authHTTPExclude`** 包含 `api`、`metrics`、`pprof` 时，这些 action **不会**再走 HTTP 鉴权（与 MediaMTX 默认行为一致）。因此本仓库配置下，**访问 `:9997` 控制 API、`:9998` 指标往往无需账号密码**——**切勿**将上述端口暴露到公网；生产环境应限制监听地址、防火墙或反代。

排查示例：

```bash
docker compose logs mediamtx --tail 100
curl -s http://localhost:9997/v3/paths/list | jq
```

路径是否就绪可在返回 JSON 中查看各 path 的 **`ready`** / **`source`** 等字段。

**Hooks：** `pathDefaults` 里的 `runOn*` 会访问 `session-api` 的 `/internal/mediamtx/hooks/*`。若注释掉 hooks，**`GET /sessions/{id}`** 里的 `input_ready`、`output_ready`、`viewer_count` 等**不会**随 MediaMTX 自动更新；流是否可用仍以 MediaMTX 与推流状态为准。

**常见端口：** `8554` RTSP · `8889` WebRTC(WHIP/WHEP) · `8189` UDP/TCP WebRTC · `9997` API · `9998` metrics · `3478` TURN（coturn）。

---

## 8) 停止与会话删除

**整栈关闭：**

```bash
docker compose down
```

**仅删除会话（Redis 记录删除，令牌立即作废）：**

```bash
curl -s -X DELETE http://localhost:8080/sessions/<session_id> | jq
```

注意：**`DELETE` 会从 Redis 移除该会话**；之后 `GET /sessions/{id}` 为 **404**。

---

## 9) 常见问题

1. **WHEP 404 / no stream**  
   先确认 WHIP 已在 `.../live/whip` 成功，且 `session_id` 与创建会话时一致；令牌与会话未过期。

2. **401 / 鉴权失败**  
   密码须为创建时返回的 **`publish.password` / `playback.password`**；用户名与 `.env` 中 `WHIP_USERNAME` / `WHEP_USERNAME` 一致。令牌过期或 **`DELETE` 会话**后需重新 `POST /sessions`。

3. **`MEDIAMTX_WEBRTC_BASE_URL` 与浏览器不同机**  
   若在手机/另一台电脑打开测试页，需把 `.env` 里该项改为**该浏览器能访问到的**宿主机 IP 或域名（含端口 **8889**），**重建** `session-api` 后再 `POST /sessions` 获取新 URL。

4. **浏览器打不开摄像头**  
   测试页须通过 **`http://` 或 `https://`** 打开，不要用 **`file://`**。

5. **TURN / ICE**  
   检查 `coturn/turnserver.conf` 与 `mediamtx.yml` 中 ICE；跨 NAT/公网需正确配置 `external-ip` 等。

6. **控制 API / Metrics 暴露**  
   默认配置下 **api/metrics** 不经 session-api 令牌鉴权；请勿把 **`:9997` / `:9998`** 暴露到不可信网络。
