#!/usr/bin/env bash
#
# gen-config.sh —— 交互式生成 service-edge 生产可用 config.yaml
#
# 自动为 agent_api_token / jwt_secret 生成 64 位随机密钥（openssl 或 /dev/urandom），
# 其余字段交互式录入并带合理默认值。生成的文件含敏感信息，权限设为 600。
#
# 用法:
#   ./scripts/gen-config.sh [输出路径]      # 默认 ./config.yaml
#
set -euo pipefail

OUT="${1:-config.yaml}"

# 解析仓库根目录（依赖 Dockerfile / .cnb.yml 的相对位置）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DOCKERFILE_PATH="$REPO_ROOT/Dockerfile"
CNB_YML_PATH="$REPO_ROOT/.cnb.yml"

# ---------- 工具函数 ----------

# 生成 n 字节随机数的十六进制（2n 个字符）
rand_hex() {
  local n="$1"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$n"
  elif [ -r /dev/urandom ]; then
    head -c "$n" /dev/urandom | od -An -tx1 | tr -d ' \n'
  else
    echo "错误: 找不到随机源 (openssl / /dev/urandom)" >&2
    exit 1
  fi
}

# 带默认值的输入；提示走 stderr，取值走 stdout 以便命令替换捕获
ask() {
  local prompt="$1" default="${2:-}" reply
  if [ -n "$default" ]; then
    read -r -p "$prompt [$default]: " reply </dev/tty || true
    printf '%s' "${reply:-$default}"
  else
    read -r -p "$prompt: " reply </dev/tty || true
    printf '%s' "$reply"
  fi
}

# 是/否确认，默认 N
confirm() {
  local prompt="$1" reply
  read -r -p "$prompt [y/N]: " reply </dev/tty || true
  [[ "$reply" =~ ^[Yy]$ ]]
}

# 查找 service-edge 可执行文件：PATH → bin/ → 输出空
find_service_edge() {
  if command -v service-edge >/dev/null 2>&1; then
    echo "service-edge"
    return 0
  fi
  if [ -x "$REPO_ROOT/bin/service-edge" ]; then
    echo "$REPO_ROOT/bin/service-edge"
    return 0
  fi
  return 1
}

# YAML 字符串转义（处理双引号与反斜杠）
yq_str() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '"%s"' "$s"
}

echo "==========================================="
echo " service-edge 配置生成向导"
echo "==========================================="
echo

# ---------- 覆盖确认 ----------
if [ -e "$OUT" ]; then
  if ! confirm "文件 $OUT 已存在，是否覆盖?"; then
    echo "已取消。" >&2
    exit 1
  fi
fi

# ---------- server ----------
LISTEN=$(ask "监听地址 (server.listen)" "0.0.0.0:8443")
# 提取端口号，同步到 Dockerfile 和 CNB YAML 中的端口映射
LISTEN_PORT="${LISTEN##*:}"
if ! [[ "$LISTEN_PORT" =~ ^[0-9]+$ ]] || [ "$LISTEN_PORT" -lt 1 ] || [ "$LISTEN_PORT" -gt 65535 ]; then
  echo "错误: 无法从监听地址 '$LISTEN' 提取有效端口号 (1-65535)" >&2
  exit 1
fi

sync_port() {
  local file="$1" port="$2"
  if [ ! -f "$file" ]; then
    echo "   (${file} 不存在，跳过端口同步)"
    return
  fi
  # 写之前先备份
  cp "$file" "${file}.bak"
  if [[ "$file" == *.cnb.yml ]] || [[ "$file" == *.yml ]]; then
    sed -i "s/-p [0-9]\+:[0-9]\+/-p ${port}:${port}/" "$file"
  elif [[ "$file" == Dockerfile* ]]; then
    sed -i "s/EXPOSE [0-9]\+/EXPOSE ${port}/" "$file"
  fi
  echo ">> 已同步 ${file}: 端口 -> ${port}（备份: ${file}.bak）"
}
sync_port "$DOCKERFILE_PATH" "$LISTEN_PORT"
sync_port "$CNB_YML_PATH" "$LISTEN_PORT"

EXTERNAL_URL=$(ask "对外地址 (server.external_url)" "https://edge-api.dreamreflex.com")

# ---------- database ----------
DB_PATH=$(ask "数据库路径 (database.path)" "/var/lib/service-edge/data.db")

# ---------- 自动生成密钥 ----------
echo
echo ">> 正在生成安全密钥..."
AGENT_TOKEN=$(rand_hex 32)   # 64 hex chars
JWT_SECRET=$(rand_hex 32)    # 64 hex chars
echo "   agent_api_token / jwt_secret 已自动生成 (各 64 字符)。"

# ---------- PKI ----------
echo
CA_CERT=$(ask "CA 证书路径 (pki.ca_cert)" "/etc/service-edge/ca.crt")
CA_KEY=$(ask "CA 私钥路径 (pki.ca_key)" "/etc/service-edge/ca.key")
# 本地暂存目录，用于存放生成的 CA（与 config.yaml 中的路径解耦）
LOCAL_CA_CERT=""
LOCAL_CA_KEY=""
CA_STAGING_DIR="${REPO_ROOT}/dev"

if [ ! -f "$CA_CERT" ] || [ ! -f "$CA_KEY" ]; then
  echo "   提示: CA 文件 ($CA_CERT / $CA_KEY) 当前不存在。"
  echo "         控制面启动会强校验 CA 材料，缺少将 panic 退出。"
  if confirm "是否现在生成测试 CA 证书?"; then
    mkdir -p "$CA_STAGING_DIR"

    local_ca_cert="${CA_STAGING_DIR}/$(basename "$CA_CERT")"
    local_ca_key="${CA_STAGING_DIR}/$(basename "$CA_KEY")"

    se_cmd=""
    if se_cmd=$(find_service_edge); then
      echo ">> 使用 $se_cmd 生成测试 CA 到 $CA_STAGING_DIR ..."
      "$se_cmd" gen-ca --out "$CA_STAGING_DIR"
    elif [ -f "$REPO_ROOT/Makefile" ] && grep -q 'dev-certs' "$REPO_ROOT/Makefile" 2>/dev/null; then
      echo ">> 使用 make dev-certs 生成测试 CA ..."
      (cd "$REPO_ROOT" && make dev-certs)
    elif command -v go >/dev/null 2>&1; then
      echo ">> 使用 go run 生成测试 CA ..."
      (cd "$REPO_ROOT" && go run ./cmd/server gen-ca --out "$CA_STAGING_DIR")
    else
      echo "错误: 未找到 service-edge / make / go，无法生成 CA。" >&2
      echo "      请手动运行: service-edge gen-ca --out $CA_STAGING_DIR" >&2
    fi

    if [ -f "$local_ca_cert" ] && [ -f "$local_ca_key" ]; then
      echo ">> CA 材料已生成到 $CA_STAGING_DIR (config.yaml 保持远程路径不变)"
      LOCAL_CA_CERT="$local_ca_cert"
      LOCAL_CA_KEY="$local_ca_key"
    else
      echo "警告: CA 生成似乎未成功，启动前请确保 CA 文件就位。" >&2
    fi
  fi
fi

# ---------- frp ----------
echo
FRP_BASE=$(ask "frp 下载基址 (frp_release.base_url)" "https://github.com/fatedier/frp/releases/download")
FRP_VERSION=$(ask "frp 默认版本 (frp_release.default_version)" "v0.61.1")

# ---------- 安装脚本 / agent 下载基址（默认从 external_url 推导）----------
echo
INSTALL_BASE=$(ask "安装脚本基址 (install_script_base)" "${EXTERNAL_URL%/}/install")
AGENT_DL_BASE=$(ask "Agent 下载基址 (agent_download_base)" "${EXTERNAL_URL%/}/download/agent")

# ---------- enrollment ----------
ENROLL_TTL=$(ask "一次性安装 token 有效期 (enrollment_token_ttl)" "15m")

# ---------- CORS ----------
echo
CORS_RAW=$(ask "前端来源白名单 (cors.allowed_origins，逗号分隔)" "https://edge.dreamreflex.com")

# ---------- logging ----------
echo
LOG_LEVEL=$(ask "日志级别 (logging.level)" "info")
LOG_PATH=$(ask "日志路径 (logging.path)" "/var/log/service-edge/server.log")

# ---------- bootstrap admin ----------
echo
ADMIN_USER=$(ask "初始管理员用户名 (bootstrap_admin.username)" "admin")
ADMIN_PWD=""
ADMIN_PWD_GENERATED=0
if confirm "自动生成强随机管理员密码?"; then
  ADMIN_PWD=$(rand_hex 12)   # 24 hex chars
  ADMIN_PWD_GENERATED=1
else
  while :; do
    read -r -s -p "请输入管理员密码: " p1 </dev/tty; echo
    read -r -s -p "请再次输入确认: " p2 </dev/tty; echo
    if [ -z "$p1" ]; then echo "密码不能为空。"; continue; fi
    if [ "$p1" != "$p2" ]; then echo "两次输入不一致，请重试。"; continue; fi
    ADMIN_PWD="$p1"
    break
  done
fi

# ---------- 组装 CORS 列表 ----------
CORS_YAML=""
IFS=',' read -r -a _origins <<< "$CORS_RAW"
for o in "${_origins[@]}"; do
  o="$(echo "$o" | xargs)"   # trim
  [ -z "$o" ] && continue
  CORS_YAML+=$'\n'"    - $(yq_str "$o")"
done
if [ -z "$CORS_YAML" ]; then
  CORS_YAML=$'\n'"    - $(yq_str "https://edge.dreamreflex.com")"
fi

# ---------- 写文件（先写临时再原子替换，权限 600）----------
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
cat > "$TMP" <<EOF
server:
  listen: $(yq_str "$LISTEN")
  external_url: $(yq_str "$EXTERNAL_URL")

database:
  path: $(yq_str "$DB_PATH")

# Agent ↔ 控制面 API 全局认证 token（自动生成，64 字符）
agent_api_token: $(yq_str "$AGENT_TOKEN")

# 用户登录 JWT 签名密钥（自动生成，64 字符）
jwt_secret: $(yq_str "$JWT_SECRET")

pki:
  ca_cert: $(yq_str "$CA_CERT")
  ca_key: $(yq_str "$CA_KEY")

frp_release:
  base_url: $(yq_str "$FRP_BASE")
  default_version: $(yq_str "$FRP_VERSION")

install_script_base: $(yq_str "$INSTALL_BASE")
agent_download_base: $(yq_str "$AGENT_DL_BASE")

enrollment_token_ttl: $(yq_str "$ENROLL_TTL")

cors:
  allowed_origins:${CORS_YAML}

logging:
  level: $(yq_str "$LOG_LEVEL")
  path: $(yq_str "$LOG_PATH")

bootstrap_admin:
  username: $(yq_str "$ADMIN_USER")
  password: $(yq_str "$ADMIN_PWD")
EOF

mv "$TMP" "$OUT"
trap - EXIT
chmod 600 "$OUT"

# ---------- 远程推送 ----------
REMOTE_CONF="$REPO_ROOT/.remote-deploy.conf"

load_remote_conf() {
  if [ -f "$REMOTE_CONF" ]; then
    # shellcheck source=/dev/null
    . "$REMOTE_CONF"
    return 0
  fi
  return 1
}

save_remote_conf() {
  cat > "$REMOTE_CONF" <<EOF
# Remote deploy config - auto-generated by gen-config.sh
# Contains SSH connection details; do not commit.
SSH_HOST='${SSH_HOST}'
SSH_USER='${SSH_USER}'
SSH_PORT='${SSH_PORT}'
SSH_KEY='${SSH_KEY}'
REMOTE_PATH='${REMOTE_PATH}'
EOF
  chmod 600 "$REMOTE_CONF"
}

push_to_remote() {
  echo
  echo "--- 远程推送配置文件 ---"

  if load_remote_conf; then
    echo ">> 已找到保存的远程配置:"
    echo "   地址: $SSH_HOST"
    echo "   用户: $SSH_USER"
    echo "   端口: $SSH_PORT"
    echo "   私钥: ${SSH_KEY:-"(使用 ssh-agent)"}"
    echo "   路径: $REMOTE_PATH"
    echo
    if ! confirm "使用已保存的远程配置?"; then
      SSH_HOST=""
    fi
  fi

  if [ -z "${SSH_HOST:-}" ]; then
    SSH_HOST=$(ask "远程服务器地址")
    if [ -z "$SSH_HOST" ]; then
      echo "未提供服务器地址，跳过推送。"
      return
    fi
    SSH_USER=$(ask "SSH 用户名" "root")
    SSH_PORT=$(ask "SSH 端口" "22")
    SSH_KEY=$(ask "SSH 私钥路径（留空使用默认 ssh-agent）" "")
    REMOTE_PATH=$(ask "远程目标路径" "/etc/service-edge/")
    save_remote_conf
    echo ">> 远程配置已保存到 $REMOTE_CONF"
  fi

  # 确保远程路径以 / 结尾
  [[ "$REMOTE_PATH" != */ ]] && REMOTE_PATH="${REMOTE_PATH}/"

  # 构建 SSH/SCP 参数
  SSH_OPTS=(-p "${SSH_PORT}")
  SCP_OPTS=(-P "${SSH_PORT}")
  if [ -n "${SSH_KEY:-}" ]; then
    SSH_OPTS+=(-i "${SSH_KEY}")
    SCP_OPTS+=(-i "${SSH_KEY}")
  fi

  # 确保远程目录存在
  echo
  echo ">> 确保远程目录存在: ${REMOTE_PATH}"
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${SSH_HOST}" "mkdir -p ${REMOTE_PATH}" || {
    echo "错误: 无法创建远程目录 ${REMOTE_PATH}" >&2
    return 1
  }

  # ---------- CA 材料准备 ----------
  # config.yaml 中的 CA 路径是运行时的目标路径；
  # 本地 CA 存放在 CA_STAGING_DIR，推送时发送到 config.yaml 指定的远程路径。
  local_cert="${LOCAL_CA_CERT:-$CA_CERT}"
  local_key="${LOCAL_CA_KEY:-$CA_KEY}"

  # PKI 阶段可能已生成，这里作为二次确认
  if [ ! -f "$local_cert" ] || [ ! -f "$local_key" ]; then
    echo
    echo "   CA 材料在本地不存在。"
    if confirm "是否自动生成测试 CA 证书并推送到远程?"; then
      mkdir -p "$CA_STAGING_DIR"
      local_cert="${CA_STAGING_DIR}/$(basename "$CA_CERT")"
      local_key="${CA_STAGING_DIR}/$(basename "$CA_KEY")"

      se_cmd=""
      if se_cmd=$(find_service_edge); then
        echo ">> 使用 $se_cmd 生成测试 CA 到 $CA_STAGING_DIR ..."
        "$se_cmd" gen-ca --out "$CA_STAGING_DIR" || {
          echo "错误: CA 生成失败" >&2
          return 1
        }
      elif [ -f "$REPO_ROOT/Makefile" ] && grep -q 'dev-certs' "$REPO_ROOT/Makefile" 2>/dev/null; then
        echo ">> 使用 make dev-certs 生成测试 CA ..."
        (cd "$REPO_ROOT" && make dev-certs) || {
          echo "错误: make dev-certs 失败" >&2
          return 1
        }
      elif command -v go >/dev/null 2>&1; then
        echo ">> 使用 go run 生成测试 CA ..."
        (cd "$REPO_ROOT" && go run ./cmd/server gen-ca --out "$CA_STAGING_DIR") || {
          echo "错误: go run gen-ca 失败" >&2
          return 1
        }
      else
        echo "错误: 未找到 service-edge / make / go，无法生成 CA。" >&2
        return 1
      fi
    fi
  fi

  # 确保远程 CA 目录存在（按 config.yaml 中的路径创建）
  remote_ca_dir="$(dirname "$CA_CERT")"
  remote_key_dir="$(dirname "$CA_KEY")"
  # 去掉 REMOTE_PATH 尾部斜杠做比较
  _rp="${REMOTE_PATH%/}"
  if [ "$remote_ca_dir" != "$_rp" ]; then
    echo ">> 远程 CA 目录 ($remote_ca_dir) 与 REMOTE_PATH ($_rp) 不同，将按 config.yaml 中的路径创建。"
  fi
  ssh "${SSH_OPTS[@]}" "${SSH_USER}@${SSH_HOST}" "mkdir -p ${remote_ca_dir} ${remote_key_dir}" || {
    echo "错误: 无法创建远程 CA 目录" >&2
    return 1
  }

  echo
  # 1. 推送 config.yaml
  echo ">> 推送 config.yaml 到 ${REMOTE_PATH}config.yaml"
  scp "${SCP_OPTS[@]}" "$OUT" "${SSH_USER}@${SSH_HOST}:${REMOTE_PATH}config.yaml" || {
    echo "错误: config.yaml 推送失败" >&2
    return 1
  }

  # 2. 推送 CA 证书到 config.yaml 指定的路径
  if [ -f "$local_cert" ]; then
    echo ">> 推送 CA 证书: $local_cert -> ${SSH_USER}@${SSH_HOST}:${CA_CERT}"
    scp "${SCP_OPTS[@]}" "$local_cert" "${SSH_USER}@${SSH_HOST}:${CA_CERT}" || {
      echo "警告: CA 证书推送失败，请手动处理" >&2
    }
  else
    echo "   (CA 证书 $local_cert 在本地不存在，跳过)"
  fi

  # 3. 推送 CA 私钥到 config.yaml 指定的路径
  if [ -f "$local_key" ]; then
    echo ">> 推送 CA 私钥: $local_key -> ${SSH_USER}@${SSH_HOST}:${CA_KEY}"
    scp "${SCP_OPTS[@]}" "$local_key" "${SSH_USER}@${SSH_HOST}:${CA_KEY}" || {
      echo "警告: CA 私钥推送失败，请手动处理" >&2
    }
  else
    echo "   (CA 私钥 $local_key 在本地不存在，跳过)"
  fi

  echo ">> 推送完成。"
}

# 询问是否推送
echo
if confirm "是否推送配置文件到远程服务器?"; then
  push_to_remote || true
fi

echo
echo "==========================================="
echo " 配置已生成: $OUT (权限 600)"
echo "==========================================="
if [ "$ADMIN_PWD_GENERATED" -eq 1 ]; then
  echo " 初始管理员: $ADMIN_USER"
  echo " 初始密码  : $ADMIN_PWD"
  echo " (请妥善记录；该密码仅首次启动创建用户时使用)"
fi
echo " 注意:"
echo "   - 文件含明文密钥，切勿提交到 git (已在 .gitignore 中忽略 config.yaml)"
echo "   - 启动前确认 CA 文件就绪: $CA_CERT / $CA_KEY"
echo "   - 启动: service-edge --config $OUT"
echo "==========================================="
