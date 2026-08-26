## How Kopia Backs Up Files (Library-Level)

### 1. Files Are Split into Variable-Size Chunks (Content-Defined Chunking)

When Kopia backs up a file, it doesn't store the file as one blob. It uses **Content-Defined Chunking (CDC)** with a rolling hash algorithm to split the file into variable-size chunks.

The default algorithm is **`DYNAMIC-4M-BUZHASH`**, which means:
- **Average chunk size:** ~4 MB
- **Minimum chunk size:** ~2 MB (`avg / 2`)
- **Maximum chunk size:** ~8 MB (`avg * 2`)

The rolling hash (Buzhash32) slides a 64-byte window across the file's bytes. When `(hash & mask) == 0`, a chunk boundary is declared. This is the key property that makes incremental backups efficient — chunk boundaries are determined by the **content of the data itself**, not by fixed byte offsets.

### 2. Every Chunk Gets a Content-Addressed ID

Each chunk is hashed with **BLAKE2B-256-128** (a keyed cryptographic hash). The resulting hash becomes the chunk's **content ID**. This is pure content-addressing: identical bytes always produce the identical ID.

From `repo/content/content_manager.go`:

```go
var hashOutput [hashing.MaxHashSize]byte

contentID, err := IDFromHash(prefix, bm.hashData(hashOutput[:0], data))
// ...
_, bi, err := bm.getContentInfoReadLocked(ctx, contentID)
// ...
if err == nil {
    if !bi.Deleted {
        bm.deduplicatedContents.Add(1)
        bm.deduplicatedBytes.Add(int64(data.Length()))
        return contentID, nil   // <-- already exists, transfer NOTHING
    }
}
return contentID, bm.addToPackUnlocked(ctx, contentID, data, false, comp, previousWriteTime, mp)
```

If the content ID already exists in the repository index, the chunk is **skipped entirely** — zero bytes transferred.

### 3. What Gets Transferred in Each Scenario

Here's the key part you can explain to users:

#### Scenario A: File doesn't change at all
**Data transferred: 0 bytes.**

Kopia first checks file metadata (mtime, size, mode, owner) against the previous snapshot. If metadata matches, it **reuses the previous object ID** without even reading the file. The file is logged as "cached."

```go
if cachedEntry := u.maybeIgnoreCachedEntry(ctx, findCachedEntry(ctx, entryRelativePath, entry, prevDirs, policyTree)); cachedEntry != nil {
    atomic.AddInt32(&u.stats.CachedFiles, 1)
    // ...
```

The file isn't opened, isn't read, isn't hashed. Completely free.

#### Scenario B: File partially changes (e.g., edit a few lines in the middle of a large file)
**Data transferred: only the changed chunk(s).**

Because CDC uses content-defined boundaries (rolling hash), chunk boundaries are anchored to the data itself. If you modify bytes in the middle of a 100 MB file:

- Chunks **before** the edit have the same bytes, same hash, same content ID — **deduplicated, not transferred**.
- The chunk(s) **containing** the edit will have different bytes, producing a new content ID — **these are transferred** (typically one or two chunks of ~4 MB each).
- Chunks **after** the edit: the rolling hash re-synchronizes within ~64 bytes past the edit boundary. So chunks downstream quickly regain their original boundaries and content IDs — **deduplicated, not transferred**.

**Net result for a small edit in a 100 MB file: ~4-8 MB transferred** (one or two chunks), not 100 MB.

This is the crucial advantage of CDC over fixed-size chunking. With fixed offsets, inserting a single byte would shift every subsequent chunk boundary, invalidating all downstream chunks. With CDC, only the local region is affected.

#### Scenario C: File completely changes (e.g., replaced with entirely new content)
**Data transferred: the full file size.**

Every chunk has new content, new hashes, new content IDs. Nothing deduplicates. The entire file is uploaded, split into ~4 MB chunks, each stored as a new content blob.

#### Scenario D: File is a copy of another file already in the repo
**Data transferred: 0 bytes.**

Since deduplication is content-addressed across the entire repository, if you copy `fileA` to `fileB`, all of `fileB`'s chunks will have the same content IDs as `fileA`'s. Every chunk is already present. Nothing new is stored.

---

### Summary Table for Users

| Scenario | File Read? | Data Transferred | Why |
|---|---|---|---|
| Unchanged file | No | 0 | Metadata cache hit from previous snapshot |
| Small edit in large file | Yes | ~1-2 chunks (~4-8 MB) | CDC boundaries resync; only affected chunks are new |
| Large edit / rewrite | Yes | ~proportional to changed region | New chunks for changed regions; unchanged regions dedup |
| Completely new content | Yes | Full file | All chunks are new |
| Duplicate of existing file | Yes | 0 | All chunks already exist (content-addressed) |

### The Object Model

At the library level, the layers are:

1. **Splitter** (`repo/splitter/`) — CDC algorithm that decides chunk boundaries
2. **Object Writer** (`repo/object/object_writer.go`) — feeds file bytes through the splitter, flushes each chunk to the content layer
3. **Content Manager** (`repo/content/`) — hashes chunks, checks for duplicates, packs new chunks into encrypted pack files
4. **Object ID** (`repo/object/objectid.go`) — small files get a single direct content ID; large files get an **indirect object** (a JSON manifest listing all chunk IDs), prefixed with `I`

So when you use the library to back up a file, you're creating an `ObjectWriter`, streaming the file into it, and letting these layers handle chunking, hashing, deduplication, and storage automatically. The amount of data actually written to the repository backend is only the bytes that are genuinely new.
