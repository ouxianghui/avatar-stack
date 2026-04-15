# Avatar Stack 启动文档

**目录：** §1 Docker（基础栈 / SoulX GPU / 自检） · §2 session-api · §3 创建会话 · §4 WHIP · §5 WHEP · §6 MediaMTX 排查 · §7 停止 · §8 常见问题

下文「**仓库根目录**」指本仓库克隆后的顶层目录；示例命令默认在仓库根执行。

---

## 1) Docker：基础栈与可选 SoulX（GPU）

### 1.0 基础栈（默认，无 PyTorch / 无 SoulX 权重）

```bash
cp .env.example .env
docker compose up -d --build
```

查看状态：

```bash
docker compose ps
```

**对外端口：**

| 服务 | 地址 |
|------|------|
| session-api | `http://localhost:8080` |
| MediaMTX WebRTC（WHIP/WHEP） | `http://localhost:8889` |
| MediaMTX RTSP | `rtsp://localhost:8554` |
| MediaMTX 控制 API（paths 等） | `http://localhost:9997`（Basic：`api` / `api-pass`，与 `mediamtx/mediamtx.yml` 中 `authInternalUsers` 一致） |

默认 `soulx-worker` 使用 **`worker/Dockerfile`**：镜像小，**不含** SoulX / PyTorch；`docker-compose.yml` 里 `SOULX_DRY_RUN=1` 时，即使会话为 `soulx` 模式也只是占位进程，**不会**加载模型。

### 1.1 SoulX-FlashHead GPU 栈（合并 compose）

在同一套服务里用 **`Dockerfile.soulx`**、`gpus: all`、挂载子模块与权重时：

```bash
git submodule update --init worker/third_party/SoulX-FlashHead
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.soulx.yml up -d --build
```

**前置条件（摘要）：**

| 项 | 说明 |
|----|------|
| GPU | 宿主机：NVIDIA 驱动 + [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)；`docker run --rm --gpus all … nvidia-smi` 能列出 GPU |
| 子模块 | `worker/third_party/SoulX-FlashHead` 已初始化 |
| 权重 | 默认使用子模块内 `models/SoulX-FlashHead-1_3B` 与 `models/wav2vec2-base-960h`（见 [SoulX-FlashHead README](https://github.com/Soul-AILab/SoulX-FlashHead)）；`docker-compose.soulx.yml` 将子模块只读挂到容器内 `/app/third_party/SoulX-FlashHead` |
| 基础镜像 | `Dockerfile.soulx` 默认 `public.ecr.aws/docker/library/python:3.10-slim`，减轻 Docker Hub 不稳定 |

合并后 `soulx-worker`：**`SOULX_DRY_RUN=0`**（可用 `.env` 覆盖）、**`TORCHDYNAMO_DISABLE=1`**（减轻 slim 镜像下 `torch.compile`/Triton 链 `-lcuda` 问题；会关闭 dynamo 编译优化，推理可能更慢）。

**多文件 compose**：后面的 `-f` 与前面的**合并**，同名服务字段以后者为准；**不是**整份替换。停止时 `-f` 列表需与启动时一致（见 §7）。

走 SoulX 管线时，创建会话见 **§3**，请求体里使用 **`"worker_mode":"soulx"`**，或在 `.env` 中设置 `WORKER_MODE=soulx`。

### 1.2 可选：SoulX 推理自检（容器外一条命令）

需已 **`docker build -f worker/Dockerfile.soulx -t avatar-soulx-worker:gpu ./worker`**，且权重在子模块 `models/` 下。

**烟测（推荐，不依赖官方 `examples/` 大文件）：**

```bash
docker run --rm --gpus all \
  -v "$(pwd)/worker/third_party/SoulX-FlashHead:/app/third_party/SoulX-FlashHead:ro" \
  -e SOULX_REPO_ROOT=/app/third_party/SoulX-FlashHead \
  -e SOULX_CKPT_DIR=/app/third_party/SoulX-FlashHead/models/SoulX-FlashHead-1_3B \
  -e SOULX_WAV2VEC_DIR=/app/third_party/SoulX-FlashHead/models/wav2vec2-base-960h \
  avatar-soulx-worker:gpu python -m app.verify_soulx_inference
```

**上游整段生成脚本**（需仓库内存在 `examples/girl.png`、`examples/podcast_sichuan_16k.wav` 等；否则先按 SoulX 文档准备示例）：

```bash
docker run --rm --gpus all -e TORCHDYNAMO_DISABLE=1 \
  -v "$(pwd)/worker/third_party/SoulX-FlashHead:/app/third_party/SoulX-FlashHead" \
  -w /app/third_party/SoulX-FlashHead \
  avatar-soulx-worker:gpu bash inference_script_single_gpu_lite.sh
```

---

## 2) 启动 session-api

### 方式 A：随 compose 启动（推荐）

执行 §1 的 `docker compose up …` 后，`session-api` 已包含在内。

```bash
docker compose logs -f session-api
```

### 方式 B：本地跑 Go（只起依赖）

```bash
docker compose up -d redis mediamtx coturn
```

```bash
cd session-api
REDIS_URL=redis://localhost:6379/0 \
MEDIAMTX_WEBRTC_BASE_URL=http://localhost:8889 \
MEDIAMTX_RTSP_BASE_URL=rtsp://localhost:8554 \
WHIP_USERNAME=publisher \
WHIP_PASSWORD=publisher-pass \
WHEP_USERNAME=viewer \
WHEP_PASSWORD=viewer-pass \
WORKER_RTSP_USER=worker \
WORKER_RTSP_PASS=worker-pass \
go run ./cmd/session-api
```

---

## 3) 创建会话（WHIP/WHEP 前置）

先创建会话再推 WHIP / 拉 WHEP。

**默认 passthrough（ffmpeg 转发 `/in` → `/out`）：**

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar","worker_mode":"passthrough"}' | jq
```

**SoulX 模式**（需 §1.1 的 GPU worker 且 `SOULX_DRY_RUN=0`）：

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar","worker_mode":"soulx"}' | jq
```

返回中会包含 `publish.whip_url`、`playback.whep_url` 及对应用户名密码。

**顺序建议：** 先 `POST /sessions` → **先 WHIP 推 `/in` 并保持** → 等 worker 在 `/out` 发布（通常数秒）→ 再 WHEP。`/out` 由 worker 生成，不是建会话就立刻有流。

---

## 4) WHIP 推流

### 方式 A：FFmpeg 推 WHIP

```bash
SESSION_ID='<上一步返回的 session_id>'

ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
  -c:a libopus -ar 48000 -ac 2 -b:a 128k \
  -f whip "http://publisher:publisher-pass@localhost:8889/avatar/${SESSION_ID}/in/whip"
```

若提示 `Requested output format 'whip' is not known`，说明本机 FFmpeg **无 WHIP muxer**（`ffmpeg -formats | grep -i whip` 无输出）。请改用 **方式 B**，或参考仓库根目录 `README.md` 的 RTSP 推流示例。

### 方式 B：浏览器 `tools/whip_push_test.html`

1. 勿用 `file://` 打开。在 `tools` 目录启动 HTTP 服务后访问：
   ```bash
   cd tools && python3 -m http.server 8765
   ```
   打开 `http://localhost:8765/whip_push_test.html`。
2. 填入当前会话的 `whip_url`、用户名、密码（与 `whep_url` 同一 `session_id`）。
3. 点击 `Start WHIP`，确认日志为 `WHIP connected`，ICE 为 `connected` / `completed`。
4. 保持页面推流，再开 WHEP。

---

## 5) WHEP 拉流

### 方式 A：浏览器 `tools/whep_test.html`

与 §4 方式 B 相同，在 `http://localhost:8765/` 下打开 `whep_test.html`，填入同会话的 `whep_url` 与凭据；WHIP 已稳定后再 `Start WHEP`。

### 方式 B：ffplay 播 RTSP `/out`

```bash
SESSION_ID='<上一步返回的 session_id>'

ffplay -rtsp_transport tcp \
  "rtsp://viewer:viewer-pass@127.0.0.1:8554/avatar/${SESSION_ID}/out"
```

---

## 6) MediaMTX hooks 与 paths API（排查）

### 6.1 `pathDefaults` 中的 hook

`mediamtx/mediamtx.yml` 的 `pathDefaults` 里配置了 `runOn*`，由 MediaMTX 调 `curl` 访问 `session-api`（`$MTX_PATH` 由 MediaMTX 展开）：

| 配置项 | 含义 |
|--------|------|
| `runOnReady` / `runOnNotReady` | path 可读流出现 / 消失 |
| `runOnRead` / `runOnUnread` | 有客户端读 / 停止读 |

`session-api` 据此更新 Redis 中会话的 `input_ready`、`output_ready`、`viewer_count` 等。**流是否真正可用**仍以 MediaMTX + worker 为准；hook 便于观测与 API 状态。

本仓库 **mediamtx** 使用自定义 `mediamtx/Dockerfile`（带 `curl`）。若改用 **无 shell/curl** 的官方 scratch 镜像，需自行删除或改写 `runOn*`。

### 6.2 在宿主机模拟 hook

```bash
SID='<session_id>'

curl -fsS -X POST "http://localhost:8080/internal/mediamtx/hooks/on-ready?path=avatar/${SID}/in"
curl -fsS -X POST "http://localhost:8080/internal/mediamtx/hooks/on-not-ready?path=avatar/${SID}/in"
curl -fsS -X POST "http://localhost:8080/internal/mediamtx/hooks/on-read?path=avatar/${SID}/out"
curl -fsS -X POST "http://localhost:8080/internal/mediamtx/hooks/on-unread?path=avatar/${SID}/out"
```

再 `GET http://localhost:8080/sessions/${SID}` 对照字段变化。

### 6.3 paths API 查看 `avatar/...`

```bash
curl -fsS -u api:api-pass "http://localhost:9997/v3/paths/list?page=0&itemsPerPage=500" \
  | jq '.items[] | select(.name|test("avatar/")) | {name,available,online,source}'
```

单条 path（替换 `SESSION_ID`）：

```bash
SESSION_ID='<session_id>'

curl -fsS -u api:api-pass "http://localhost:9997/v3/paths/list?page=0&itemsPerPage=500" \
  | jq --arg p "avatar/${SESSION_ID}/in" '.items[] | select(.name == $p) | {name,available,online,source}'

curl -fsS -u api:api-pass "http://localhost:9997/v3/paths/list?page=0&itemsPerPage=500" \
  | jq --arg p "avatar/${SESSION_ID}/out" '.items[] | select(.name == $p) | {name,available,online,source}'
```

`/in` 上 `available: true` 且 `source` 有 `type`，通常表示发布已挂上；`/out` 同理才说明 worker 已发布，WHEP 不易报「no stream」。

若注释掉 `mediamtx.yml` 里指向 `session-api` 的 hook，`GET /sessions/{id}` 中的部分字段**不会**随 MediaMTX 自动更新；排障以 **paths API + worker 日志** 为准。

---

## 7) 停止与清理

```bash
curl -s -X DELETE "http://localhost:8080/sessions/<session_id>" | jq
```

仅基础栈：

```bash
docker compose down
```

曾用 SoulX 覆盖文件启动时，**使用相同 `-f` 列表**：

```bash
docker compose -f docker-compose.yml -f docker-compose.soulx.yml down
```

---

## 8) 常见问题：WHEP / `/out` 无流

含义：MediaMTX 上 **`avatar/<id>/out` 尚无发布中的流**。

1. **未对该 `session_id` 调用 `POST /sessions`**  
   只有创建会话时才会向 Redis 发 **start**，worker 才会等 `/in` 并推 `/out`。只对随机 UUID 做 WHIP、从未建会话，则 `/out` 不会有 worker。

2. **WHIP 与 WHEP 的 `session_id` 不一致**  
   两页 URL 须来自同一次 `POST /sessions`。可用  
   `http://localhost:8765/whip_push_test.html?session=<uuid>`  
   与  
   `http://localhost:8765/whep_test.html?session=<uuid>`。

3. **`/in` 无真实推流**  
   确认 WHIP 已连接；并用 **§6.3** 的 paths API 查 `avatar/${SESSION_ID}/in` 的 `source`。

4. **worker 异常或未重建**  
   ```bash
   docker compose ps
   docker compose logs soulx-worker --tail 80
   ```  
   passthrough 正常时日志中常见 `publisher present on /in`、`starting session=... mode=passthrough`；SoulX 栈则为 `mode=soulx`（且不应长期停在 dry-run 占位）。  
   更新 worker 代码或 compose 后需重建对应服务，例如：  
   - 仅基础栈：`docker compose up -d --build soulx-worker`  
   - 含 SoulX：`docker compose -f docker-compose.yml -f docker-compose.soulx.yml up -d --build soulx-worker`

5. **重启过 worker 或 Redis 后仍用旧 `session_id`**  
   `start` 消息通常只消费一次；重启后请重新 `POST /sessions` 使用新的 `session_id`。

再查 `/out`：

```bash
curl -s -u api:api-pass "http://localhost:9997/v3/paths/list" \
  | jq --arg p "avatar/${SESSION_ID}/out" '.items[] | select(.name == $p)'
```
