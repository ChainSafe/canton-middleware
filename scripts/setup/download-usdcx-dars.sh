#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CACHE_DIR="${USDCX_DAR_CACHE_DIR:-${ROOT_DIR}/deployments/usdcx-dars}"

UTILITY_BUNDLE_VERSION="0.12.0"
UTILITY_BUNDLE="canton-network-utility-dars-${UTILITY_BUNDLE_VERSION}.tar.gz"
UTILITY_BASE_URL="https://get.digitalasset.com/utility-dars"

XRESERVE_DAR="utility-bridge-v0-0.1.3.dar"
XRESERVE_BASE_URL="https://get.digitalasset.com/usdc-dars"

mkdir -p "${CACHE_DIR}/utility"

download() {
  local url="$1"
  local dest="$2"
  if [ -f "$dest" ]; then
    echo "exists: ${dest}"
    return
  fi
  echo "download: ${url}"
  curl -fsSL "$url" -o "$dest"
}

download "${UTILITY_BASE_URL}/${UTILITY_BUNDLE}" "${CACHE_DIR}/${UTILITY_BUNDLE}"
download "${UTILITY_BASE_URL}/${UTILITY_BUNDLE}.sha256" "${CACHE_DIR}/${UTILITY_BUNDLE}.sha256"
download "${XRESERVE_BASE_URL}/${XRESERVE_DAR}" "${CACHE_DIR}/${XRESERVE_DAR}"
download "${XRESERVE_BASE_URL}/${XRESERVE_DAR}.sha256" "${CACHE_DIR}/${XRESERVE_DAR}.sha256"

(
  cd "${CACHE_DIR}"
  shasum -a 256 -c "${UTILITY_BUNDLE}.sha256"
  shasum -a 256 -c "${XRESERVE_DAR}.sha256"
)

tar -xzf "${CACHE_DIR}/${UTILITY_BUNDLE}" -C "${CACHE_DIR}/utility"

cat > "${CACHE_DIR}/manifest.json" <<JSON
{
  "utility_bundle": "${UTILITY_BUNDLE}",
  "xreserve_dar": "${XRESERVE_DAR}",
  "packages": {
    "utility_bridge_v0": "efbff60554e834835e3e75ecfc8675adf27521996982d195d527f7c1b0840bf6",
    "utility_registry_app_v0": "7a75ef6e69f69395a4e60919e228528bb8f3881150ccfde3f31bcc73864b18ab",
    "utility_registry_v0": "a236e8e22a3b5f199e37d5554e82bafd2df688f901de02b00be3964bdfa8c1ab",
    "utility_registry_holding_v0": "8107899ac4723ce986bf7d27416534e576e54b92161e46150a595fb78ff3d3a1",
    "utility_credential_v0": "5a29ead611a0abd5f5b3fc3caf7d0f67c0ff802032ab6d392824aa9060e56d70",
    "splice_api_token_holding_v1": "718a0f77e505a8de22f188bd4c87fe74101274e9d4cb1bfac7d09aec7158d35b",
    "splice_api_token_transfer_instruction_v1": "55ba4deb0ad4662c4168b39859738a0e91388d252286480c7331b3f71a517281",
    "splice_api_token_metadata_v1": "4ded6b668cb3b64f7a88a30874cd41c75829f5e064b3fbbadf41ec7e8363354f"
  }
}
JSON

echo "USDCx DAR cache ready: ${CACHE_DIR}"
