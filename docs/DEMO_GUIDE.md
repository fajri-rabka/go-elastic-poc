# Demo Guide — Go + Elasticsearch POC (5–10 Menit)

Panduan praktis ini disiapkan untuk demo langsung kepada **Koh Franz** atau engineer lain tanpa harus membaca seluruh riwayat commit dan catatan historis proyek.

---

## 1. Project Objective

POC (*Proof of Concept*) ini membuktikan bahwa **Elasticsearch dapat digunakan sebagai primary datastore untuk aplikasi Go backend**, menyimpan dokumen JSON secara langsung tanpa ketergantungan pada MySQL ataupun PostgreSQL.

> **Bukan untuk:** Logging pipeline, Elastic APM, OpenTelemetry, distributed tracing, ataupun Kibana dashboard.

---

## 2. Arsitektur Singkat

```text
HTTP Client (curl / Postman)
         │
         ▼
     Go API (cmd/api)
         │
         ▼
     Handler (internal/handler)
         │  - HTTP routing (Go 1.22+ ServeMux)
         │  - Strict JSON decoding (DisallowUnknownFields, 1 MiB limit)
         │  - HTTP status & error mapping (400, 404, 409, 503, 500)
         ▼
     Service (internal/service)
         │  - Domain business validation (Brand must be Honda, cc > 0, price > 0)
         │  - Pagination window guard (from + limit <= 10,000)
         │  - Context-aware sort defaulting (relevance vs name_asc)
         ▼
    Repository (internal/repository/elastic)
         │  - Official Elasticsearch Go Typed Client v9
         │  - Explicit strict mapping verification
         │  - Optimistic concurrency control (seq_no & primary_term)
         │  - Complex Bool Query builder (multi_match, term, range)
         ▼
  Elasticsearch 9.5.5 (motorcycles index)
         │
         ▼
    JSON Document (Honda Motorcycle Catalog)
```

---

## 3. Persiapan & Menjalankan Elasticsearch

Buka terminal dan jalankan container Elasticsearch via Docker Compose:

```bash
# Jalankan container di background
make up
# atau: docker compose up -d

# Tunggu sampai status container berubah menjadi healthy
make ps
# atau: docker compose ps
```

*Verifikasi kesehatan cluster:*

```bash
make health
# atau: curl -s http://localhost:9200/_cluster/health | jq .
```

*Expected output:* Cluster status `green` atau `yellow` (di setup single-node kita, status adalah `green` karena replicas diatur 0).

---

## 4. Menjalankan Go REST API

Buka terminal baru (atau jalankan di tab terpisah):

```bash
make run
# atau: go run ./cmd/api
```

*Expected log:*
```text
level=INFO msg="connected to elasticsearch" cluster=docker-cluster version=9.5.5
level=INFO msg="index ready (existing mapping verified)" index=motorcycles
level=INFO msg="REST API server listening" addr=http://127.0.0.1:8080
```

> **Bootstrap & Mapping Safety:**
> Baik `make run` (`cmd/api`) maupun `make demo` (`cmd/demo`) secara otomatis memanggil `repo.EnsureIndex()`. Jika index `motorcycles` belum ada (misal pada fresh clone), aplikasi langsung membuatnya dengan **explicit strict mapping** (1 shard, 0 replica, `dynamic: strict`, `model_name.keyword` subfield) sebelum melayani request. Aplikasi tidak pernah mengandalkan dynamic auto-mapping Elasticsearch.

---

## 5. Verifikasi Health Endpoint

Pastikan API server sudah merespons:

```bash
curl -i http://127.0.0.1:8080/health
```

*Expected response:*
```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok"}
```

---

## 6. Create Honda Motorcycle (Khusus Demo)

> **Catatan Keamanan Data:** Kita menggunakan ID demo khusus: `honda-cbr250-demo-001`. Jangan gunakan ID `honda-beat-001` (dokumen verifikasi Phase 3).

*Cek apakah ID sudah terpakai:*

```bash
curl -i http://127.0.0.1:8080/motorcycles/honda-cbr250-demo-001
# Harus return 404 Not Found sebelum demo dimulai
```

*Kirim request pembuatan motor baru:*

```bash
curl -i -X POST http://127.0.0.1:8080/motorcycles \
  -H "Content-Type: application/json" \
  -d '{
    "id": "honda-cbr250-demo-001",
    "brand": "Honda",
    "model_name": "CBR250RR SP Quick Shifter",
    "category": "sport",
    "engine_cc": 250,
    "transmission": "manual",
    "price_idr": 79000000,
    "available": true,
    "description": "Motor sport fairing 250cc 2 silinder dengan fitur assist slipper clutch dan quick shifter"
  }'
```

*Expected response:* `HTTP/1.1 201 Created`

---

## 7. Get Motorcycle by ID (Real-time Read)

Operasi GET by `_id` di Elasticsearch bersifat **real-time** (membaca langsung dari segment/translog terbaru tanpa menunggu refresh):

```bash
curl -i http://127.0.0.1:8080/motorcycles/honda-cbr250-demo-001
```

*Expected response:* `HTTP/1.1 200 OK` dengan payload detail dokumen.

---

## 8. Update Motorcycle (Full Replacement & Concurrency Lock)

Lakukan perubahan harga dan spesifikasi menggunakan `PUT`:

```bash
curl -i -X PUT http://127.0.0.1:8080/motorcycles/honda-cbr250-demo-001 \
  -H "Content-Type: application/json" \
  -d '{
    "brand": "Honda",
    "model_name": "CBR250RR SP Quick Shifter",
    "category": "sport",
    "engine_cc": 250,
    "transmission": "manual",
    "price_idr": 81500000,
    "available": true,
    "description": "Motor sport fairing 250cc 2 silinder dengan penyesuaian harga OTR terbaru"
  }'
```

*Expected response:* `HTTP/1.1 200 OK` (harga berubah menjadi `81500000`).

---

## 9. Full-Text Search dengan Relevansi

Cari motor berdasarkan kata kunci `"Quick Shifter"` di deskripsi dan nama:

```bash
curl -s "http://127.0.0.1:8080/motorcycles?search=Quick+Shifter" | jq .
```

*Catatan Teknis:* Elasticsearch menggunakan `multi_match` dengan boosting `model_name^2`. Jika kata kunci cocok di nama model, skor relevansi (`_score`) akan berbobot lebih tinggi daripada kemunculan di deskripsi.

---

## 10. Structured Filter (Category & Availability)

Filter katalog untuk kategori `"sport"` yang berstatus `"available=true"`:

```bash
curl -s "http://127.0.0.1:8080/motorcycles?category=sport&available=true" | jq .
```

*Catatan Teknis:* Filter ini dieksekusi di dalam **Filter Context** (`bool.filter`), sehingga tidak menghitung score dan memenuhi syarat untuk di-cache (*cache-eligible*) oleh bitset node query cache Elasticsearch jika dieksekusi berulang kali pada segment yang berukuran memadai.

---

## 11. Numeric Range & Sorting (Price)

Cari motor sport dengan rentang harga Rp 50.000.000 sampai Rp 90.000.000, diurutkan dari yang termahal:

```bash
curl -s "http://127.0.0.1:8080/motorcycles?category=sport&min_price=50000000&max_price=90000000&sort=price_desc" | jq .
```

*Catatan Teknis:* Pengurutan teks (nama model) menggunakan sub-field `model_name.keyword` pada struktur columnar disk (**doc_values**) untuk menghindari pemborosan RAM `fielddata`.

---

## 12. Safe Pagination & Deep Pagination Guard

Halaman pertama dengan limit 1:

```bash
curl -s "http://127.0.0.1:8080/motorcycles?page=1&limit=1" | jq .
```

*Tunjukkan proteksi Deep Pagination (Guard 10.000):*

```bash
curl -i "http://127.0.0.1:8080/motorcycles?page=1001&limit=10"
```

*Expected response:*
```http
HTTP/1.1 400 Bad Request

{"error":"page offset and limit exceed maximum result window of 10000"}
```
*Poin penting untuk Koh Franz:* Service Go secara proaktif mencegah request yang melebihi batas default `max_result_window` Elasticsearch agar cluster tidak terkena Out-Of-Memory (OOM).

---

## 13. Cleanup Dokumen Demo

Hapus dokumen yang tadi dibuat khusus demo:

```bash
curl -i -X DELETE http://127.0.0.1:8080/motorcycles/honda-cbr250-demo-001
```

*Expected response:* `HTTP/1.1 204 No Content`

*Pastikan dokumen sudah terhapus:*

```bash
curl -i http://127.0.0.1:8080/motorcycles/honda-cbr250-demo-001
# Return 404 Not Found
```

---

## 14. Bukti Data Tersimpan di Elasticsearch (Verifikasi Engine)

Tunjukkan ke Koh Franz bahwa dokumen sample Phase 3 (`honda-beat-001`) masih tersimpan utuh di storage engine Elasticsearch:

```bash
# Cek langsung ke REST API Elasticsearch port 9200
curl -s http://localhost:9200/motorcycles/_doc/honda-beat-001 | jq .
```

Dan jalankan runner demo Phase 3:

```bash
make demo
```

*Expected output:* Status `already exists, mapping verified`, data cocok 100% dan idempotent.

---

## 15. Trade-off & Limitasi untuk Diskusi Teknis

Saat sesi tanya jawab dengan Koh Franz, jelaskan poin-poin arsitektural berikut:

1. **Near-Real-Time (NRT) vs Real-Time:**
   - GET by ID bersifat real-time via translog.
   - Search API membutuhkan index refresh (default interval 1 detik) agar dokumen baru terlihat di inverted index.
2. **Ketiadaan Transaksi ACID Multi-Document:**
   - Elasticsearch tidak memiliki transaksi multi-tabel/multi-dokumen layaknya PostgreSQL. Operasi atomik hanya berlaku di level single document.
3. **Optimistic Concurrency Control:**
   - Menggunakan `_seq_no` dan `_primary_term` untuk mendeteksi tabrakan write di level repository. Di production masa depan, client HTTP dapat mengirim header `If-Match` (ETag) untuk mendeteksi stale edits antar client.
4. **Peran Ideal di Production:**
   - Untuk katalog produk search-heavy, Elasticsearch sangat kuat sebagai search index terdistribusi. Namun jika sistem membutuhkan transaksi finansial ketat (misal order/payment ledger), arsitektur terbaik adalah PostgreSQL sebagai primary datastore dengan CDC (Change Data Capture) / Kafka sink ke Elasticsearch.

