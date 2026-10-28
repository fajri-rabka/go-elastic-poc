# Troubleshooting — go-elasticsearch-poc

Catatan masalah yang **pernah terjadi** atau **relevan** dengan setup kita (Windows + Docker Desktop + WSL2 Ubuntu).

Tiap masalah dikasih label:

- **TERBUKTI** — pernah kejadian dan penyebabnya sudah dikonfirmasi.
- **TERJADI, PENYEBAB BELUM PASTI** — gejalanya pernah muncul, tapi root cause masih dugaan.
- **HIPOTESIS** — belum pernah terbukti, tapi masuk akal buat dicek.
- **PENCEGAHAN** — belum pernah kejadian, dicatat buat jaga-jaga.

> Aturan main: **jangan naikin timeout, kill proses orang, atau ubah setting WSL/kernel cuma buat "nutupin" gejala.** Cari tahu dulu penyebabnya.

---

## Alur debugging cepat

Kerjain dari atas. Begitu ketemu langkah yang gagal, fokus di situ.

```bash
# 1. Docker kebaca di WSL?
docker compose version

# 2. Container jalan dan healthy?
docker compose ps

# 3. Elasticsearch jawab (tanpa Go)?
curl --max-time 10 -w '\nHTTP %{http_code} in %{time_total}s\n' http://localhost:9200

# 4. Resource cukup?
free -h
docker stats --no-stream

# 5. Baru Go
go run ./cmd/api
```

| Hasil | Arah investigasi |
|---|---|
| Langkah 1 gagal | [Docker nggak tersedia di WSL](#1-docker-nggak-tersedia-di-wsl) |
| Langkah 2 nggak `healthy` | [Container unhealthy](#10-elasticsearch-container-unhealthy) |
| Langkah 3 `Couldn't connect` | [Connection refused](#2-elasticsearch-connection-refused) |
| Langkah 3 lama / timeout | [Memory pressure](#7-memory-pressure), [context deadline exceeded](#4-context-deadline-exceeded) |
| Langkah 3 sukses, langkah 5 gagal | [context deadline exceeded](#4-context-deadline-exceeded), [Proxy](#5-proxy-environment), cek `echo $ELASTICSEARCH_URL` |

---

## 1. Docker nggak tersedia di WSL

**Label:** TERBUKTI (pernah terjadi di Phase 1, dan **terjadi lagi 2026-10-28**).

**Gejala:**

```text
The command 'docker' could not be found in this WSL 2 distro.
We recommend to activate the WSL integration in Docker Desktop settings.
```

**Penyebab yang mungkin:**
- Docker Desktop di Windows belum jalan / sudah ditutup.
- WSL Integration buat distro Ubuntu dimatiin.

**Cara cek:**

```bash
ls -la /usr/bin/docker            # biasanya symlink ke /mnt/wsl/docker-desktop/cli-tools/...
ls /mnt/wsl/docker-desktop        # kalau "No such file or directory" → Docker Desktop belum nyambung ke distro ini
```

Pada 2026-10-28, `/usr/bin/docker` masih ada sebagai symlink, tapi targetnya (`/mnt/wsl/docker-desktop/...`) nggak ada. Artinya CLI-nya terpasang, tapi Docker Desktop lagi nggak nyambung.

**Cara fix yang aman:**
1. Buka Docker Desktop di Windows, tunggu sampai statusnya running.
2. Kalau masih gagal: *Docker Desktop → Settings → Resources → WSL Integration* → aktifin distro Ubuntu → *Apply & restart*.
3. Buka terminal WSL baru, cek lagi `docker compose version`.

> `wsl --shutdown` dari PowerShell juga bisa bantu, tapi itu **matiin semua distro WSL** (termasuk proses lain yang lagi jalan, misal dev server). Pakai kalau memang perlu.

---

## 2. Elasticsearch connection refused

**Label:** TERBUKTI (2026-10-28, karena Elasticsearch nggak jalan).

**Gejala:**

```text
error: check elasticsearch connection to http://localhost:9200: an error happened during the Info query execution: dial tcp 127.0.0.1:9200: connect: connection refused
```

atau dari curl:

```text
curl: (7) Failed to connect to localhost port 9200 after 0 ms: Couldn't connect to server
```

**Penyebab yang mungkin:**
- Container Elasticsearch nggak jalan (atau Docker Desktop mati).
- Container masih booting.
- `ELASTICSEARCH_URL` nunjuk ke port/host yang salah.

**Kenapa ini penting dibedain dari timeout:** *Refused* artinya OS langsung bilang "nggak ada yang listen di port ini". Cepet banget (di test kita: 0.008 detik). Jadi masalahnya bukan lambat, tapi memang nggak ada server.

**Cara cek:**

```bash
docker compose ps
echo "${ELASTICSEARCH_URL:-<unset, pakai default http://localhost:9200>}"
ss -ltn | grep 9200
```

**Cara fix yang aman:**

```bash
make up                      # atau: docker compose up -d
docker compose ps            # tunggu sampai healthy
curl http://localhost:9200
unset ELASTICSEARCH_URL      # kalau ternyata ada nilai nyasar di shell
```

---

## 3. Cluster yellow vs green

**Label:** PENCEGAHAN (yang pernah tercatat: `green`).

**Gejala:** `_cluster/health` nunjukin `"status": "yellow"`.

**Penyebab yang mungkin:** Di single node, Elastic **nggak pernah** naruh replica shard di node yang sama dengan primary-nya. Index dengan `number_of_replicas >= 1` (default-nya 1) → replica *unassigned* → `yellow`.

**Cara cek:**

```bash
curl "http://localhost:9200/_cluster/health?pretty"
curl "http://localhost:9200/_cat/shards?v"
curl "http://localhost:9200/_cluster/allocation/explain?pretty"   # kalau ada unassigned shard
```

**Cara fix yang aman:**
- `yellow` di local single node itu **normal**. Read/write tetap jalan karena semua primary aman.
- Kalau mau `green` buat index POC (Phase 3 nanti), set `number_of_replicas: 0` di index tersebut. Itu keputusan **local only**.
- `red` beda cerita: ada primary shard yang nggak tersedia → cek logs.

Healthcheck di `compose.yml` nganggep `yellow` dan `green` sehat, dan gagal kalau ES nggak bisa diakses atau `red`.

---

## 4. `context deadline exceeded`

**Label:** TERJADI, PENYEBAB BELUM PASTI.

**Gejala (session sebelumnya):**

```text
error: check elasticsearch connection to http://localhost:9200: an error happened during the Info query execution: context deadline exceeded
```

Padahal `curl http://localhost:9200` bisa.

**Apa artinya secara teknis:** Go nunggu jawaban sampai batas `connectionTimeout` (10 detik) di `cmd/api/main.go`, dan nggak dapet response lengkap.
Kita sudah buktiin behavior ini di 2026-10-28 dengan IP yang sengaja nggak bisa dijangkau (`10.255.255.1`): error yang sama muncul tepat di **10.02 detik**. Jadi timeout-nya bekerja sesuai desain.

**Petunjuk penting:** Di localhost, kalau nggak ada yang listen, hasilnya *connection refused* secara instan, **bukan** timeout. Jadi waktu timeout terjadi di session sebelumnya, kemungkinan besar ada yang **nerima koneksi TCP** (Docker Desktop port forwarder / Elasticsearch) tapi **nggak sempat ngirim jawaban** dalam 10 detik. Ini cocok dengan dugaan sistem lagi kehabisan resource.

**Penyebab yang mungkin (urut dari yang paling didukung bukti):**
1. **Memory pressure / swap thrashing** di VM WSL2 — HIPOTESIS KUAT. Waktu itu swap 2 GB hampir penuh, RAM free cuma ~70–90 MB, dan ES ngeluarin warning `transport_worker did not make progress`. Lihat [bagian 7](#7-memory-pressure) dan [8](#8-swap-thrashing).
2. Kebetulan timing: curl dan Go dijalanin di momen yang beda, sementara kondisi sistem naik-turun — HIPOTESIS.
3. Proxy — **kecil kemungkinannya** untuk URL localhost. Lihat [bagian 5](#5-proxy-environment).

**Cara cek (jalanin berurutan, di terminal yang sama, dalam waktu berdekatan):**

```bash
free -h
cat /proc/pressure/memory          # angka avg10/avg60 tinggi = sistem lagi nunggu memory
docker stats --no-stream

curl -sS --max-time 10 -w '\nHTTP %{http_code} in %{time_total}s\n' http://localhost:9200
time go run ./cmd/api

# Bandingin dengan 127.0.0.1
ELASTICSEARCH_URL=http://127.0.0.1:9200 go run ./cmd/api

# Cari warning di ES
docker compose logs --tail=200 elasticsearch | grep -i -E "warn|did not make progress|OutOfMemory"
```

Tip: `go run` itu compile dulu baru jalan. Kalau mau ngukur koneksinya doang, build dulu:

```bash
go build -o bin/api ./cmd/api && time ./bin/api
```

**Cara fix yang aman:**
- Jangan langsung naikin timeout. 10 detik buat `GET /` di localhost itu sudah longgar banget; kalau masih kurang, ada yang salah di environment.
- Bebasin memory dulu (lihat bagian 7), lalu ulang tes.
- Kalau curl juga lambat di momen yang sama → masalahnya di ES/Docker/resource, bukan di Go.
- Kalau curl selalu cepat tapi Go selalu timeout di kondisi memory yang lega → itu baru tanda masalah di sisi Go, dan perlu diinvestigasi lebih dalam (misal pakai `curl -v` vs log transport client).

---

## 5. Proxy environment

**Label:** HIPOTESIS (session sebelumnya nggak ketemu proxy aktif; 2026-10-28 juga nggak ada env proxy).

**Gejala:** Go atau curl gagal / lambat, padahal ES jalan.

**Fakta penting:**
- Client Elasticsearch pakai clone dari `http.DefaultTransport`, yang baca `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`.
- **Tapi** Go selalu **nggak** pakai proxy untuk host `localhost` dan IP loopback (`127.0.0.1`). Ini dicek langsung di source Go (`net/http/httpproxy`, fungsi `useProxy`).
- Jadi untuk `ELASTICSEARCH_URL=http://localhost:9200`, proxy **nggak bisa** jadi penyebab timeout di Go.
- Proxy baru relevan kalau URL-nya bukan loopback, misal IP Windows host atau hostname lain.
- curl punya aturan sendiri soal proxy. Biar yakin curl nggak lewat proxy, pakai `--noproxy '*'`.

**Cara cek:**

```bash
env | grep -i proxy || echo "no proxy env"
curl --noproxy '*' --max-time 10 http://localhost:9200
```

**Cara fix yang aman:** Kalau URL bukan loopback dan ada proxy, tambahin host-nya ke `NO_PROXY` **di shell lu sendiri**. Jangan ubah konfigurasi proxy sistem/perusahaan.

---

## 6. Localhost resolution

**Label:** HIPOTESIS (2026-10-28: `localhost` resolve ke `127.0.0.1` saja).

**Gejala:** `localhost` gagal tapi `127.0.0.1` berhasil (atau sebaliknya).

**Penyebab yang mungkin:** `localhost` bisa resolve ke IPv4 (`127.0.0.1`) dan IPv6 (`::1`). Kalau port forwarding cuma jalan di salah satunya, hasilnya bisa beda-beda.

**Cara cek:**

```bash
getent ahosts localhost
curl --max-time 10 http://localhost:9200
curl --max-time 10 http://127.0.0.1:9200
```

Error Go juga nunjukin IP yang dipakai, misal `dial tcp 127.0.0.1:9200`.

**Cara fix yang aman:** Pakai IP eksplisit lewat env var, tanpa ubah code:

```bash
ELASTICSEARCH_URL=http://127.0.0.1:9200 go run ./cmd/api
```

---

## 7. Memory pressure

**Label:** TERJADI (memory pernah sangat ketat), tapi **belum terbukti** sebagai penyebab timeout.

**Gejala:**
- Semua terasa lambat, compile Go lama.
- ES ngeluarin warning `transport_worker did not make progress`.
- Request yang biasanya milidetik jadi detik-an atau timeout.

**Kenapa ini relevan banget di setup kita:** Dengan Docker Desktop backend WSL2, container Elasticsearch jalan di **VM WSL2 yang sama** dengan distro Ubuntu lu. Jadi `free -h` di Ubuntu sebenernya nunjukin memory VM yang dipakai bareng: Elasticsearch (heap 1 GB + overhead), `next-server`, `gopls`, Node, compiler Go, dll. Satu proses gede bisa bikin yang lain kelaparan.

Perbandingan kondisi:

| Waktu | RAM available | Swap terpakai | Catatan |
|---|---|---|---|
| Session sebelumnya | ~70–90 MB free | ~2 GB (hampir/penuh 100%) | `next-server`, `gopls`, Node, compiler Go makan RAM besar |
| 2026-10-28 | 4.3 GiB available | 120–215 MiB | ES **nggak** jalan; `next-server` ~1.6 GB RSS |

> Catatan: hari ini ES nggak jalan. Begitu ES nyala, available memory bakal turun kira-kira 1.5–2 GB.

**Cara cek:**

```bash
free -h
cat /proc/pressure/memory
ps -eo pid,rss,comm --sort=-rss | head -15
docker stats --no-stream
```

**Cara fix yang aman (lu yang mutusin, jangan otomatis):**
1. Tutup sendiri proses berat yang lagi nggak dipakai (misal dev server Next.js project lain).
2. Matiin Elasticsearch kalau lagi nggak ngerjain project ini: `make down` (data tetap aman di volume). Perlu diingat `restart: unless-stopped` bikin ES ikut nyala tiap Docker Desktop start.
3. Opsional, keputusan lu: atur memory WSL2 lewat `%UserProfile%\.wslconfig` (`[wsl2]` → `memory=...`), lalu `wsl --shutdown`.
4. Last resort: turunin heap ES ke `-Xms512m -Xmx512m` di `compose.yml` (ini perubahan config, perlu disepakati dulu).

---

## 8. Swap thrashing

**Label:** HIPOTESIS KUAT untuk timeout di session sebelumnya, belum terbukti.

**Pengertian:** Kalau RAM penuh, Linux mindahin halaman memory yang jarang dipakai ke swap (disk). *Thrashing* terjadi kalau sistem terus-terusan bolak-balik mindahin data RAM ↔ swap, jadi waktunya habis buat mindahin data, bukan ngerjain kerjaan. Akibatnya proses yang sebenarnya ringan (kayak ES jawab `GET /`) bisa nyangkut berdetik-detik.

**Gejala:** Swap hampir penuh, sistem kerasa "freeze" sesaat, load naik, request timeout secara acak.

**Cara cek:**

```bash
free -h
vmstat 1 5                     # kolom si/so (swap in/out) yang terus tinggi = tanda thrashing
cat /proc/pressure/memory      # nilai "full" yang naik = semua task sempat nunggu memory
```

**Cara fix yang aman:** Sama dengan [bagian 7](#7-memory-pressure). Nambah swap cuma bikin thrashing lebih lama, bukan nyelesain akar masalah.

---

## 9. `vm.max_map_count`

**Label:** PENCEGAHAN (belum pernah kejadian; 2026-10-28 nilainya sudah `1048576`).

**Gejala:** Container ES mati saat boot, log berisi:

```text
max virtual memory areas vm.max_map_count [65530] is too low, increase to at least [1048576]
```

**Penyebab:** Elasticsearch butuh banyak memory-mapped area buat Lucene. Batas default Linux terlalu kecil.

**Cara cek:**

```bash
sysctl vm.max_map_count
docker compose logs --tail=200 elasticsearch | grep -i max_map_count
```

Semua distro WSL2 (termasuk VM Docker Desktop) berbagi kernel yang sama, jadi nilai dari Ubuntu ini sudah mencerminkan kernel yang dipakai container.

**Cara fix yang aman (hanya kalau error itu beneran muncul):**

```bash
sudo sysctl -w vm.max_map_count=1048576     # sementara, sampai reboot/wsl --shutdown
```

Kalau mau permanen di Linux:

```bash
echo 'vm.max_map_count=1048576' | sudo tee /etc/sysctl.d/99-elasticsearch.conf
sudo sysctl --system
```

Alternatif di Docker Desktop (dari PowerShell):

```powershell
wsl -d docker-desktop -u root sysctl -w vm.max_map_count=1048576
```

Project ini **nggak pernah** ngubah setting kernel secara otomatis.

---

## 10. Elasticsearch container unhealthy

**Label:** PENCEGAHAN.

**Gejala:** `docker compose ps` nunjukin `health: starting` terus, `unhealthy`, atau container restart terus.

**Penyebab yang mungkin:**
- Masih booting (ada `start_period: 60s`, healthcheck dicoba tiap 10 detik sampai 12 kali).
- Memory nggak cukup (exit code `137`, `OutOfMemoryError`).
- `vm.max_map_count` kerendahan.
- Cluster `red`.
- Port `9200` sudah dipakai proses lain.

**Cara cek:**

```bash
docker compose ps -a
docker inspect --format '{{json .State.Health}}' go-elasticsearch-poc-es
docker compose logs --tail=200 elasticsearch
ss -ltnp | grep 9200                       # port bentrok?
```

Cari kata `ERROR`, `bootstrap check failure`, `OutOfMemoryError` di logs.

**Cara fix yang aman:**
- Tunggu dulu sampai `start_period` lewat.
- Kalau port bentrok (`bind: address already in use`), matiin proses/container lain yang pakai `9200`, lalu `make up`.
- Kalau memory: lihat [bagian 7](#7-memory-pressure). Di Docker Desktop, alokasi minimal 2 GB, idealnya 4 GB.
- Kalau `max_map_count`: lihat [bagian 9](#9-vmmax_map_count).
- `restart: unless-stopped` bikin container gagal bisa restart berulang — logs yang nunjukin penyebab aslinya.

> **Jangan** langsung `make reset` (`docker compose down -v`) buat "benerin" container unhealthy. Itu **ngehapus semua data** di volume `elasticsearch_data`.

---

## 11. Index mapping mismatch / conflict

**Label:** TERBUKTI (tertangani secara aman di Phase 3).

**Gejala:** Saat menjalankan aplikasi atau demo, muncul error:
```text
error: ensure index: index "motorcycles" already exists with a different mapping and was left unchanged: ...
```

**Penyebab:** Index dengan nama yang sama sudah pernah dibuat sebelumnya (misal secara manual via curl atau dibuat dengan skema lama), dan tipe salah satu field-nya berbeda atau ada field yang hilang.

**Kenapa ini penting:** Di Elasticsearch, tipe field yang sudah terdaftar di mapping **tidak bisa diubah** tanpa membuat index baru dan melakukan reindexing. Mengubah mapping yang tidak kompatibel di index yang sedang dipakai bisa merusak query dan indexing data.

**Cara cek:**
```bash
curl -sS "http://localhost:9200/motorcycles/_mapping?pretty"
```

**Cara fix yang aman:**
1. Bandingkan output curl dengan definisi di `internal/repository/elastic/motorcycle.go`.
2. Jika ini local POC dan lu memang ingin merombak skema dari nol, gunakan nama index versi baru di config:
   ```bash
   ELASTICSEARCH_INDEX=motorcycles_v2 make demo
   ```
3. Jangan sembarangan menghapus index yang sudah ada jika belum yakin datanya tidak dibutuhkan.

---

## 12. `version_conflict_engine_exception` saat Insert

**Label:** TERBUKTI (tertangani secara aman di Phase 3).

**Gejala:** Operasi create document mengembalikan error:
```text
create motorcycle "honda-beat-001": document already exists
```

**Penyebab:** Kita menggunakan API `Create` (`r.client.Create(r.index, id)`), bukan API `Index`. API `Create` memaksakan aturan bahwa dokumen dengan `_id` tersebut harus belum ada. Jika sudah ada, Elasticsearch mengembalikan HTTP 409 Conflict.

**Kenapa ini penting:** Ini adalah mekanisme proteksi agar data tidak tertimpa (*overwrite*) secara diam-diam. Di `cmd/demo/main.go`, error ini dikenali lewat `errors.Is(err, elastic.ErrAlreadyExists)` sehingga program tidak panik dan memperlakukan run kedua sebagai idempotency verification.

**Cara cek:**
```bash
curl -sS "http://localhost:9200/motorcycles/_doc/honda-beat-001?pretty"
```

**Cara fix yang aman:**
- Jika memang ingin memperbarui data, buat dan gunakan operasi `Update` terpisah dengan payload eksplisit (Phase 4).
- Jika memang ingin membuat dokumen baru, gunakan identifier `_id` yang berbeda (misal `honda-beat-002`).

---

## 13. Port 8080 already in use (REST API Server)

**Label:** PENCEGAHAN.

**Gejala:** `make run` (`go run ./cmd/api`) gagal dengan error:
```text
bind: address already in use
```

**Penyebab:** Port `8080` sedang digunakan oleh proses lain (misal dev server lain di WSL atau container Docker lain).

**Cara cek:**
```bash
ss -ltnp | grep 8080
# atau
lsof -i :8080
```

**Cara fix yang aman:**
1. Hentikan proses yang sedang memakai port `8080`, ATAU
2. Jalankan server dengan port lain lewat env var tanpa mengubah source code:
   ```bash
   HTTP_ADDR=127.0.0.1:8081 make run
   ```

---

## 14. HTTP 503 Datastore Service Unavailable

**Label:** TERBUKTI (tertangani secara aman di Phase 4).

**Gejala:** API merespons dengan HTTP status 503:
```json
{
  "error": "datastore service unavailable"
}
```

**Penyebab:** Go API tidak dapat menghubungi Elasticsearch (misal container Elasticsearch mati, port forwarding Docker terputus, atau request timeout setelah 10 detik).

**Kenapa ini penting:** Handler mengklasifikasikan error dependency datastore dan mengembalikan HTTP 503 (bukan 500), tanpa membocorkan raw error network/credential ke client publik, sementara error internal dicatat secara terstruktur di `log/slog`.

**Cara cek:**
```bash
docker compose ps
curl http://localhost:9200
```

**Cara fix yang aman:**
Nyalakan kembali Elasticsearch (`make up`), pastikan statusnya `healthy`, lalu request API kembali.



