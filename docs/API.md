# REST API Specification — Motorcycle Catalog

Dokumentasi lengkap kontrak endpoint, format request/response JSON, status code, penanganan error, serta aturan pencarian dan pagination untuk Motorcycle Catalog API.

**Base URL:** `http://127.0.0.1:8080` (dapat dikonfigurasi melalui `HTTP_ADDR`)

---

## Daftar Endpoint

| Method | Path | Deskripsi | Status Sukses |
|---|---|---|---|
| `GET` | `/health` | Pemeriksaan kesehatan proses API server | `200 OK` |
| `POST` | `/motorcycles` | Menambahkan dokumen motor baru ke katalog | `201 Created` |
| `GET` | `/motorcycles` | Pencarian, filtering, sorting, dan pagination motor | `200 OK` |
| `GET` | `/motorcycles/{id}` | Mengambil detail motor berdasarkan document `_id` | `200 OK` |
| `PUT` | `/motorcycles/{id}` | Penggantian dokumen motor secara utuh (*full replacement*) | `200 OK` |
| `DELETE` | `/motorcycles/{id}` | Menghapus dokumen motor dari index | `204 No Content` |

---

## 1. Health Check

Memverifikasi bahwa proses Go REST API aktif dan siap melayani request.

- **URL:** `GET /health`
- **Request Headers:** None
- **Response Headers:** `Content-Type: application/json`
- **Status:** `200 OK`

```json
{
  "status": "ok"
}
```

---

## 2. Create Motorcycle

Menyimpan dokumen motor Honda baru ke Elasticsearch. Document ID ditentukan oleh client melalui field `id` pada request body JSON.

- **URL:** `POST /motorcycles`
- **Request Headers:** `Content-Type: application/json`
- **Body Limit:** Maksimal 1 MiB (`DisallowUnknownFields` aktif)
- **Status Sukses:** `201 Created`
- **Status Error:** `400 Bad Request`, `409 Conflict`, `503 Service Unavailable`

### Request Body

| Field | Tipe | Wajib | Aturan Bisnis |
|---|---|---|---|
| `id` | string | Ya | 3–64 karakter, lowercase slug alphanumeric & hyphen (contoh: `honda-cbr250-001`) |
| `brand` | string | Ya | Harus bernilai `"Honda"` |
| `model_name` | string | Ya | Teks nama model, tidak boleh kosong |
| `category` | string | Ya | Kategori motor (contoh: `"matic"`, `"sport"`, `"cub"`) |
| `engine_cc` | integer | Ya | Kapasitas mesin > 0 cc |
| `transmission` | string | Ya | Tipe transmisi (contoh: `"automatic"`, `"manual"`) |
| `price_idr` | integer (int64) | Ya | Harga dalam rupiah > 0 |
| `available` | boolean | Ya | `true` atau `false` |
| `description` | string | Ya | Deskripsi motor, tidak boleh kosong |

```json
{
  "id": "honda-scoopy-api-001",
  "brand": "Honda",
  "model_name": "Scoopy",
  "category": "matic",
  "engine_cc": 110,
  "transmission": "automatic",
  "price_idr": 22000000,
  "available": true,
  "description": "Motor matic Honda untuk kebutuhan harian"
}
```

### Response Body (`201 Created`)

```json
{
  "id": "honda-scoopy-api-001",
  "brand": "Honda",
  "model_name": "Scoopy",
  "category": "matic",
  "engine_cc": 110,
  "transmission": "automatic",
  "price_idr": 22000000,
  "available": true,
  "description": "Motor matic Honda untuk kebutuhan harian"
}
```

---

## 3. List & Search Motorcycles

Mendukung full-text search, exact filtering, numeric range, multi-field sorting, dan bounded pagination.

- **URL:** `GET /motorcycles`
- **Query Parameters:**

| Parameter | Tipe | Default | Keterangan |
|---|---|---|---|
| `search` | string | `""` | Kata kunci pencarian bebas (mencari pada `model_name^2` dan `description`) |
| `category` | string | `""` | Filter exact match kategori (contoh: `matic`, `sport`) |
| `transmission` | string | `""` | Filter exact match transmisi (contoh: `automatic`, `manual`) |
| `available` | boolean | `nil` | Filter status ketersediaan (`true` atau `false`) |
| `min_price` | integer | `nil` | Batas bawah harga (IDR), harus >= 0 dan <= `max_price` |
| `max_price` | integer | `nil` | Batas atas harga (IDR), harus >= 0 |
| `sort` | string | Contextual | Opsi pengurutan (lihat tabel aturan sorting di bawah) |
| `page` | integer | `1` | Nomor halaman (1-based, minimum 1) |
| `limit` | integer | `10` | Jumlah data per halaman (1–50) |

### Aturan Sorting

| Nilai `sort` | Perilaku Pengurutan | Keterangan |
|---|---|---|
| `relevance` | Skor relevansi BM25 (`_score desc`) | **Hanya valid** jika parameter `search` diisi. Mengirim `sort=relevance` tanpa `search` menghasilkan 400 Bad Request. |
| `price_asc` | Harga terendah ke tertinggi (`price_idr asc`) | Dieksekusi via doc_values |
| `price_desc`| Harga tertinggi ke terendah (`price_idr desc`)| Dieksekusi via doc_values |
| `name_asc`  | Nama model alfabetis A-Z (`model_name.keyword asc`)| Menggunakan keyword sub-field |
| `name_desc` | Nama model alfabetis Z-A (`model_name.keyword desc`)| Menggunakan keyword sub-field |

> **Default Sort Behavior:**
> - Jika parameter `sort` kosong dan `search` diisi: otomatis menggunakan `relevance`.
> - Jika parameter `sort` kosong dan `search` kosong: otomatis menggunakan `name_asc`.

### Aturan Pagination & Deep Pagination Guard

- Offset dihitung menggunakan rumus: $\text{from} = (\text{page} - 1) \times \text{limit}$.
- **Guard Result Window:** Nilai $\text{from} + \text{limit}$ **tidak boleh melebihi 10.000** (default `index.max_result_window` Elasticsearch).
- Jika melebihi 10.000, API mengembalikan `400 Bad Request` guna mencegah kehabisan memori (*OOM*) pada cluster Elasticsearch.

### Response Body (`200 OK`)

```json
{
  "data": [
    {
      "id": "honda-beat-001",
      "brand": "Honda",
      "model_name": "BeAT",
      "category": "matic",
      "engine_cc": 110,
      "transmission": "automatic",
      "price_idr": 19000000,
      "available": true,
      "description": "Motor matic Honda untuk mobilitas harian"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total": 1,
    "total_pages": 1
  }
}
```

> **Catatan Empty Results:** Jika tidak ada dokumen yang cocok, field `data` **selalu** berupa array kosong `[]`, tidak pernah `null`.

---

## 4. Get Motorcycle by ID

Membaca satu dokumen motor secara real-time berdasarkan `_id`.

- **URL:** `GET /motorcycles/{id}`
- **Path Parameter:** `id` (slug ID dokumen)
- **Status Sukses:** `200 OK`
- **Status Error:** `400 Bad Request`, `404 Not Found`

```bash
curl -i http://127.0.0.1:8080/motorcycles/honda-beat-001
```

---

## 5. Replace Motorcycle (PUT)

Melakukan penggantian data secara utuh (*full document replacement*) pada dokumen motor yang sudah ada. Menggunakan Optimistic Concurrency Control (`_seq_no` & `_primary_term`).

- **URL:** `PUT /motorcycles/{id}`
- **Request Headers:** `Content-Type: application/json`
- **Status Sukses:** `200 OK`
- **Status Error:**
  - `400 Bad Request`: Payload tidak lengkap / field tidak valid.
  - `404 Not Found`: Dokumen dengan ID tersebut belum ada (tidak melakukan upsert).
  - `409 Conflict`: Terjadi tabrakan versi concurrent write (`version_conflict_engine_exception`).

```json
{
  "brand": "Honda",
  "model_name": "Scoopy Prestige",
  "category": "matic",
  "engine_cc": 110,
  "transmission": "automatic",
  "price_idr": 23500000,
  "available": true,
  "description": "Motor matic Honda edisi Prestige"
}
```

---

## 6. Delete Motorcycle

Menghapus dokumen motor dari index.

- **URL:** `DELETE /motorcycles/{id}`
- **Status Sukses:** `204 No Content`
- **Status Error:** `400 Bad Request`, `404 Not Found`

```bash
curl -i -X DELETE http://127.0.0.1:8080/motorcycles/honda-scoopy-api-001
```

---

## 7. Format Error Response

Semua error dikembalikan dalam format JSON terstandarisasi:

```json
{
  "error": "pesan error yang informatif"
}
```

### Tabel Status Code

| Status Code | Kondisi Penyebab |
|---|---|
| `400 Bad Request` | Sintaks JSON rusak, unknown fields, payload > 1 MiB, validasi bisnis gagal (cc <= 0, harga <= 0, brand bukan Honda, min_price > max_price, page/limit invalid, result window > 10000). |
| `404 Not Found` | Dokumen motor dengan ID yang diminta tidak ditemukan di Elasticsearch (pada GET, PUT, atau DELETE). |
| `409 Conflict` | Mencoba membuat ID yang sudah terdaftar (POST), atau terjadi benturan konkurensi (PUT concurrent conflict). |
| `500 Internal Server Error`| Terjadi kesalahan sistem tak terduga. Detail internal tidak dibocorkan ke client. |
| `503 Service Unavailable` | Cluster Elasticsearch tidak dapat dihubungi atau mengalami timeout (*dependency failure*). |

