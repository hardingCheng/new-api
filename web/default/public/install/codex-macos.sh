#!/bin/sh

set -eu

station_base_url=${STATION_BASE_URL:-https://easycodehub.com/v1}
station_name=${STATION_NAME:-}
station_model=${STATION_MODEL:-gpt-5.6-sol}
codex_config_dir=${HOME}/.codex
codex_config_file=${codex_config_dir}/config.toml
codex_temp_file=''
terminal_echo_disabled=0

restore_terminal() {
  if [ "${terminal_echo_disabled}" -eq 1 ] && [ -r /dev/tty ]; then
    stty echo </dev/tty 2>/dev/null || true
    printf '\n' >/dev/tty
  fi
  if [ -n "${codex_temp_file}" ] && [ -f "${codex_temp_file}" ]; then
    rm -f "${codex_temp_file}"
  fi
}

trap restore_terminal EXIT HUP INT TERM

case "${station_base_url}" in
  https://*/v1) ;;
  *)
    printf '%s\n' '错误：STATION_BASE_URL 必须是以 https:// 开头、以 /v1 结尾的地址。' >&2
    exit 1
    ;;
esac

if [ -z "${station_name}" ]; then
  case "${station_base_url}" in
    https://easycodehub.com/v1) station_name='易编码杭州站' ;;
    *) station_name='Current Station' ;;
  esac
fi

case "${station_base_url}${station_name}${station_model}" in
  *'"'*|*'\'*|*'
'*)
    printf '%s\n' '错误：配置参数包含不支持的字符。' >&2
    exit 1
    ;;
esac

if [ "$(uname -s)" != 'Darwin' ]; then
  printf '%s\n' '此安装助手目前仅支持 macOS。' >&2
  exit 1
fi

codex_executable=${CODEX_EXECUTABLE:-}
if [ -z "${codex_executable}" ]; then
  for candidate in \
    '/Applications/ChatGPT.app/Contents/Resources/codex' \
    "${HOME}/Applications/ChatGPT.app/Contents/Resources/codex" \
    '/Applications/Codex.app/Contents/Resources/codex' \
    "${HOME}/Applications/Codex.app/Contents/Resources/codex"
  do
    if [ -x "${candidate}" ]; then
      codex_executable=${candidate}
      break
    fi
  done
fi

if [ -z "${codex_executable}" ] && command -v codex >/dev/null 2>&1; then
  codex_executable=$(command -v codex)
fi

if [ -z "${codex_executable}" ] || [ ! -x "${codex_executable}" ]; then
  printf '%s\n' '没有找到 Codex 桌面端。请先安装最新版 ChatGPT/Codex 桌面应用，再重新运行此命令。' >&2
  printf '%s\n' '官方下载：https://developers.openai.com/codex/quickstart?setup=app' >&2
  exit 2
fi

printf '%s\n' '易编码 Codex 桌面端配置助手'
printf '接口：%s\n模型：%s\n\n' "${station_base_url}" "${station_model}"

codex_api_key=${CODEX_API_KEY:-}
if [ -z "${codex_api_key}" ]; then
  if [ ! -r /dev/tty ]; then
    printf '%s\n' '错误：无法读取终端。请在 macOS 终端中运行安装命令。' >&2
    exit 1
  fi
  printf '%s' '请粘贴本站 API 密钥（输入不会显示）：' >/dev/tty
  stty -echo </dev/tty
  terminal_echo_disabled=1
  IFS= read -r codex_api_key </dev/tty
  stty echo </dev/tty
  terminal_echo_disabled=0
  printf '\n' >/dev/tty
fi

case "${codex_api_key}" in
  sk-*) ;;
  *)
    printf '%s\n' '错误：API 密钥应以 sk- 开头。' >&2
    exit 1
    ;;
esac

mkdir -p "${codex_config_dir}"
chmod 700 "${codex_config_dir}"

timestamp=$(date '+%Y%m%d-%H%M%S')
if [ -s "${codex_config_file}" ]; then
  backup_file=${codex_config_file}.backup-${timestamp}
  if [ -e "${backup_file}" ]; then
    backup_file=${backup_file}-$$
  fi
  cp "${codex_config_file}" "${backup_file}"
  chmod 600 "${backup_file}"
  printf '已备份原配置：%s\n' "${backup_file}"
fi

codex_temp_file=$(mktemp "${TMPDIR:-/tmp}/easycodehub-codex.XXXXXX")

cat >"${codex_temp_file}" <<EOF
# >>> station provider managed by the Codex setup helper
model_provider = "station"
model = "${station_model}"
model_reasoning_effort = "high"

[model_providers.station]
name = "${station_name}"
base_url = "${station_base_url}"
requires_openai_auth = true
wire_api = "responses"
# <<< station provider managed by the Codex setup helper
EOF

if [ -f "${codex_config_file}" ]; then
  awk '
    BEGIN { section = ""; skip_station = 0 }
    $0 ~ /^# >>> station provider managed by the Codex setup helper$/ { skip_managed = 1; next }
    $0 ~ /^# <<< station provider managed by the Codex setup helper$/ {
      skip_managed = 0
      skip_station = 0
      section = ""
      next
    }
    skip_managed { next }
    /^[[:space:]]*\[/ {
      if ($0 ~ /^[[:space:]]*\[model_providers\.station\][[:space:]]*$/) {
        skip_station = 1
        next
      }
      skip_station = 0
      section = $0
    }
    skip_station { next }
    section == "" && $0 ~ /^[[:space:]]*(model_provider|model|model_reasoning_effort)[[:space:]]*=/ { next }
    { print }
  ' "${codex_config_file}" >>"${codex_temp_file}"
fi

mv "${codex_temp_file}" "${codex_config_file}"
codex_temp_file=''
chmod 600 "${codex_config_file}"

if ! printf '%s' "${codex_api_key}" | "${codex_executable}" login --with-api-key; then
  printf '%s\n' 'API 密钥保存失败。配置文件已写入，请检查密钥后重新运行。' >&2
  exit 1
fi

codex_api_key=''

printf '\n%s\n' '配置完成。请彻底退出 ChatGPT/Codex 桌面端，然后重新打开并新建一个 Codex 任务。'
printf '配置文件：%s\n' "${codex_config_file}"
