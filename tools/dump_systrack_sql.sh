#!/usr/bin/env bash
#
# systrack.sql üretici
# --------------------
# Canlı/yerel veritabanından tek bir systrack.sql çıkarır:
#   - TÜM tabloların YAPISI (CREATE TABLE) alınır  -> her zaman güncel, tam.
#   - Sadece SEED_TABLES listesindeki tabloların VERİSİ eklenir (kodun beklediği
#     default satırlar). Diğer tabloların verisi alınmaz (uçucu/log/geçmiş verisi).
#
# Böylece taze kurulumda `mysql < systrack.sql` demen yeterli olur; elle INSERT yok.
# Yeni tablo eklenince yapısı otomatik gelir. Yeni bir DEFAULT tablo eklediysen
# yalnızca aşağıdaki SEED_TABLES listesine adını eklersin.
#
# Kullanım:
#   ./tools/dump_systrack_sql.sh
#   DB_NAME=systrack DB_USER=root DB_PASS=sifre ./tools/dump_systrack_sql.sh
#   INCLUDE_USERS=1 ./tools/dump_systrack_sql.sh   # users tablosunu da seed'e kat
#
set -euo pipefail

# --- Bağlantı ayarları (env ile geçilebilir) ---
DB_NAME="${DB_NAME:-systrack}"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-root}"
DB_PASS="${DB_PASS:-}"
OUT_FILE="${OUT_FILE:-systrack.sql}"

# --- SEED (default veri) tabloları: kodun kurulumda beklediği hazır satırlar ---
# NOT: Buraya yeni bir "default satır" tablosu eklersen otomatik olarak verisiyle çıkar.
SEED_TABLES=(
  settings
  snmp_settings
  ssh_settings
  winrm_settings
  ad_settings
  module_permissions
  notification_config
  notification_settings
  notification_templates
  new_notification_config
  new_notification_settings
  new_notification_templates
  report_templates
  inventory_scan_settings
)

# users tablosu parola hash'i içerir; repoya girecek bir dosyada olması sakıncalı.
# Gerekiyorsa INCLUDE_USERS=1 ile ekle (sonra parolayı sıfırlamayı unutma).
if [[ "${INCLUDE_USERS:-0}" == "1" ]]; then
  SEED_TABLES+=(users)
fi

# --- mysqldump / mysql araçlarını bul (XAMPP dahil) ---
find_bin() {
  local name="$1"
  if command -v "$name" >/dev/null 2>&1; then command -v "$name"; return; fi
  for p in /c/xampp/mysql/bin "/c/Program Files/MySQL/MySQL Server 8.0/bin" /usr/bin /usr/local/bin; do
    if [[ -x "$p/$name" ]]; then echo "$p/$name"; return; fi
    if [[ -x "$p/$name.exe" ]]; then echo "$p/$name.exe"; return; fi
  done
  return 1
}

MYSQLDUMP="$(find_bin mysqldump || true)"
MYSQL="$(find_bin mysql || true)"
if [[ -z "$MYSQLDUMP" || -z "$MYSQL" ]]; then
  echo "HATA: mysqldump/mysql bulunamadı. PATH'e ekleyin veya XAMPP kurulu olsun." >&2
  exit 1
fi

# Parolayı komut satırında bırakmamak için ortam değişkeni ile geç.
export MYSQL_PWD="$DB_PASS"
CONN=(-h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER")

# Ortak dump bayrakları:
#   --skip-dump-date  : çıktı her seferinde aynı olsun (git diff'i temiz kalsın)
#   --no-tablespaces  : PROCESS yetkisi gerektirmesin
#   --single-transaction : tutarlı anlık görüntü, tabloları kilitlemeden
COMMON_FLAGS=(--skip-dump-date --no-tablespaces --single-transaction --default-character-set=utf8mb4)

echo ">> Veritabanı: $DB_NAME @ $DB_HOST:$DB_PORT (kullanıcı: $DB_USER)"

# Bağlantı testi
if ! "$MYSQL" "${CONN[@]}" -N -e "USE \`$DB_NAME\`;" >/dev/null 2>&1; then
  echo "HATA: '$DB_NAME' veritabanına bağlanılamadı. Ayarları kontrol edin." >&2
  exit 1
fi

# DB'de gerçekten var olan tabloları al (seed listesini bunlarla kesiştireceğiz)
mapfile -t EXISTING_TABLES < <("$MYSQL" "${CONN[@]}" -N -e "SHOW TABLES IN \`$DB_NAME\`;")

# Seed listesinden yalnızca DB'de var olanları seç (yoksa mysqldump patlamasın)
SEED_PRESENT=()
for t in "${SEED_TABLES[@]}"; do
  for e in "${EXISTING_TABLES[@]}"; do
    if [[ "$t" == "$e" ]]; then SEED_PRESENT+=("$t"); break; fi
  done
done

TMP_STRUCT="$(mktemp)"
TMP_SEED="$(mktemp)"
trap 'rm -f "$TMP_STRUCT" "$TMP_SEED"' EXIT

echo ">> 1/2 Tüm tabloların YAPISI alınıyor (${#EXISTING_TABLES[@]} tablo)..."
"$MYSQLDUMP" "${CONN[@]}" "${COMMON_FLAGS[@]}" --no-data "$DB_NAME" > "$TMP_STRUCT"

if [[ "${#SEED_PRESENT[@]}" -gt 0 ]]; then
  echo ">> 2/2 SEED verisi alınıyor (${#SEED_PRESENT[@]} tablo): ${SEED_PRESENT[*]}"
  "$MYSQLDUMP" "${CONN[@]}" "${COMMON_FLAGS[@]}" \
    --no-create-info --skip-triggers --complete-insert --skip-extended-insert \
    "$DB_NAME" "${SEED_PRESENT[@]}" > "$TMP_SEED"
else
  echo ">> 2/2 SEED tablosu bulunamadı, veri kısmı atlanıyor."
  : > "$TMP_SEED"
fi

# --- Tek dosyada birleştir ---
{
  echo "-- ============================================================"
  echo "-- systrack.sql  (tools/dump_systrack_sql.sh ile üretildi)"
  echo "-- Yapı: TÜM tablolar | Seed veri: ${SEED_PRESENT[*]:-yok}"
  echo "-- ============================================================"
  echo "SET NAMES utf8mb4;"
  echo "SET FOREIGN_KEY_CHECKS=0;"
  echo
  cat "$TMP_STRUCT"
  echo
  echo "-- ---------- SEED (default) VERİ ----------"
  cat "$TMP_SEED"
  echo
  echo "SET FOREIGN_KEY_CHECKS=1;"
} > "$OUT_FILE"

SIZE="$(wc -c < "$OUT_FILE" | tr -d ' ')"
echo ">> Bitti: $OUT_FILE (${SIZE} bayt)"
echo ">> Yapı: ${#EXISTING_TABLES[@]} tablo | Seed veri: ${#SEED_PRESENT[@]} tablo"

# Seed içinde hassas veri barındırabilecek tablolar için uyarı
for s in "${SEED_PRESENT[@]}"; do
  case "$s" in
    users|settings)
      echo "!! UYARI: '$s' hassas veri (parola/SMTP vb.) içerebilir. Paylaşmadan önce temizle." ;;
  esac
done
