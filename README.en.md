## BSDMP (Binary Self-Describing Messaging Protocol)

i18n: [English](README.en.md) | [Russian](README.md)

BSDMP (Binary Self-Describing Messaging Protocol) is a binary protocol for efficient transmission of structured data between client and server. The protocol uses a self-describing structure that allows the recipient to interpret data without prior knowledge of its format.

---

Version: 3.0

---

## Message Structure

```c
struct BSDMPMessage {
    byte[4]  magic;           // Format signature: 'BSDM'
    BSDMPHeader header;       // Message header
    BSDMPDataBlock data;      // Data block (field headers + frames), possibly compressed
    byte[4]  crc;             // CRC of data block (regardless of compression)
}

struct BSDMPHeader {
    uint8_t version;           // Protocol version (e.g., 1)
    uint8_t extension;         // Extensions version (for custom extensions)
    uint8_t compression;       // Compression method (0 = none, 1 = gzip, etc.)
    uint8_t reserved1;         // Reserved (for alignment)
    uint8_t binary_flags[8];   // Custom bit flags (8 bytes = 64 boolean flags)
    uint32_t data_block_size;  // Size of data block in bytes (possibly compressed)
    uint8_t reserved2[8];      // Reserved (for 16-byte alignment)
}

struct BSDMPDataBlock {
    uint32_t frame_count;      // Number of frames in block
    
    uint32_t fse_len;          // Frame start marker length
    byte[fse_len] fse;         // Byte sequence - frame start
    
    uint32_t fee_len;          // Frame end marker length
    byte[fee_len] fee;         // Byte sequence - frame end

    uint32_t title_len;        // Total length of field headers
    byte[title_len] title;     // List of field descriptions (title entries)

    BSDMPFrame[frame_count] frames; // List of frames
}

(Repeats field_count times as serialized sequence within title)
struct BSDMPTitleEntry {
    uint16_t name_len;         // Field name length
    byte[name_len] name;       // Field name (UTF-8)
    uint8_t type_code;         // Data type (see type enum - e.g., int, string, binary, etc.)
    uint8_t type_len;          // Number of bytes allocated for field_len (usually 1 or 2, can be 4)
}

struct BSDMPFrame {
    byte[fse_len] fse;         // Frame start marker (repeats data.fse)

    uint32_t frame_num;        // Frame sequence number
    uint16_t field_count;      // Number of fields in frame (may be less than or equal to title_count)

    BSDMPField[field_count] fields; // List of fields

    byte[fee_len] fee;         // Frame end marker (repeats data.fee)
}

struct BSDMPField {
    uint8_t title_index;       // Index in title (0 to title_count - 1)
    byte[type_len] field_len;  // Data length, `type_len` taken from corresponding title
    byte[field_len] data;      // Field data
}
```

---

## Compression Methods

| Code | Method | Description |
| ---- | ------ | ----------- |
| 0x00 | RAW    | No compression |
| 0x01 | ZLIB   | ZLIB compression |
| 0x02 | GZIP   | GZIP compression |
| 0x03 | BZIP2  | BZIP2 compression |
| 0x04 | LZMA   | LZMA compression |

---

## Data Types

| Code | Type | Format |
| ---- | ---- | ------ |
| 0x01 | STRING | UTF-8 string |
| 0x02 | INT | int64_t (8 bytes, little-endian) |
| 0x03 | FLOAT | double (8 bytes, IEEE754) |
| 0x04 | BOOL | 1 byte (0x00/0x01) |
| 0x05 | JSON | UTF-8 JSON string |
| 0x06 | BINARY | Arbitrary binary data |

---
## Encoding Process

1. **Header Formation (BSDMPHeader):**
   * `version` - protocol version
   * `extension` - extension version
   * `compression` - selected compression method
   * `reserved` - alignment/reserved space
   * `data_block_size` - filled after data block compression
   * `reserved` - alignment/reserved space

2. **Data Block Preparation (BSDMPDataBlock):**
   * Set `frame_count` - number of frames
   * Set `fse_len` and `fee_len`, define byte markers `fse` and `fee`
   * Form `title`:
     * For each field define `BSDMPTitleEntry`: `name_len`, `name`, `type_code`, `type_len`
     * Final header is serialized with `title_len`

3. **Frame Encoding:**
   For each `BSDMPFrame`:
   * Start with `fse` marker
   * Specify `frame_num` and `field_count`
   * For each field:
     * `title_index` - field index from header
     * `field_len` - written in `type_len` bytes
     * `data` - field data in `field_len` bytes
   * End with `fee` marker

4. **Data Block Compression:**
   * Entire `BSDMPDataBlock` (from `frame_count` to last byte of last frame) is compressed according to `compression`
   * Compressed block is stored, its length written to `data_block_size`

5. **Final Message Formation (BSDMPMessage):**
   * Prefix `magic = "BSDM"`
   * Add `BSDMPHeader`
   * Add compressed `BSDMPDataBlock`
   * Calculate CRC (e.g., CRC32) from `BSDMPDataBlock` (already compressed) and append

---

## Decoding Process

1. **Header Verification and Reading:**
   * Verify first 4 bytes match `"BSDM"`
   * Read `BSDMPHeader`
   * Get `data_block_size` and extract corresponding bytes from stream

2. **Data Block Decompression:**
   * Decompress `BSDMPDataBlock` using specified `compression` method

3. **Field Header Parsing (title):**
   * Read `frame_count`, `fse_len`, `fse`, `fee_len`, `fee`
   * Read `title_len`, extract header
   * For each `BSDMPTitleEntry`:
     * Read `name_len`, `name`, `type_code`, `type_len`
     * Build mapping table (index → entry)

4. **Frame Processing:**
   For `frame_count` frames:
   * Verify `fse` at frame start
   * Read `frame_num`, `field_count`
   * For each field:
     * Read `title_index`
     * Using `type_len` from header, read `field_len`
     * Read `data` of length `field_len`
   * Verify `fee` at frame end
   * Build frame map if needed (index → field value)

5. **Integrity Check:**
   * Calculate checksum (CRC) from compressed `BSDMPDataBlock`
   * Compare with stored `crc` value

---

## Implementation Requirements

* All numeric values in **little-endian** format
* Strict adherence to all field sizes
* Mandatory boundary checks and buffer size validation during read/write
* Proper error handling and rejection of malformed structures
* Support for all specified data types, including extensible fields

---

## Limitations

* Maximum field name length: **65535 bytes**
* Default maximum field value length: **255 bytes**, **extensible via `type_len`**
* Protocol version: **3** (current)
* Main header size (`BSDMPHeader`): **24 bytes**
* Message header size (`BSDMPMessage`): **32 bytes** (4 magic + 24 header + 4 CRC)
* Support for field headers (`title_block`) with structure `name_len`, `name`, `type_code`, `type_len`
* `CRC`: 4 bytes, CRC32 of data block (before or after compression - determined by compression method)
* All frames must include `frame_num` and `field_len`
* Absence of `fse/fee` - frame start/end determined structurally

---

## Development Roadmap (Current Status)

✅ Implemented:
- `BSDM` magic block (version 3) at packet start
- `field_len` extension via `type_len` (1/2/4/8 bytes)
- Enhanced `title_block` header: `name_len`, `name`, `type_code`, `type_len`
- Support for binary data, strings, and intX types
- CRC32 in header

🔜 In Progress:
- Extended data types: `json`, `float`, `datetime`
- Parser-level structure validation: unique names, type correctness
- SDK/Parser expansion:
  - Python, Go, PHP (priority)
  - JS/TS (NodeJS, Deno, Bun)
  - C#, C++, C, Rust (future consideration)

### BSDMP Feature Matrix

| #  | Feature                                          | Status         | Version | Notes                                      |
| -- | ------------------------------------------------ | -------------- | ------- | ------------------------------------------ |
| 1  | Little-endian support for all numbers            | ✅ Implemented | v1+     | Since inception                            |
| 2  | Fixed-length header (24 bytes)                   | ✅ Implemented | v3      | Includes version, compression, data size   |
| 3  | Max name/value length = 65535                    | ✅ Implemented | v1      | Via `uint16`                               |
| 4  | `string` type (UTF-8, no null)                   | ✅ Implemented | v1      |                                            |
| 5  | `int64` type                                     | ✅ Implemented | v3      | Via `type_code` and `type_len`             |
| 6  | `bytes` type (raw data)                          | ✅ Implemented | v2      |                                            |
| 7  | `json` type (as text)                            | ✅ Implemented | v3      | Encoded as `string`                        |
| 8  | `binary` type                                    | ✅ Implemented | v3      | Arbitrary binary data                      |
| 9  | Validated `json` type                            | 🔜 Planned     | v4?     | Will encode as `string` with `type=json`   |
| 10 | `float`/`double` types                           | ✅ Implemented | v3      | Encoded as `float64`                       |
| 11 | `datetime` type                                  | 🔜 Planned     | v4?     | Likely UNIX timestamp + type indicator     |
| 12 | `BSDM` magic block                               | ✅ Implemented | v3      | Signature identifier                       |
| 13 | `type_code` field                                | ✅ Implemented | v2      | Simplifies deserialization                 |
| 14 | `type_len` field                                 | ✅ Implemented | v3      | Enables 1/2/4/8 byte `field_len`           |
| 15 | Packet-wide CRC                                  | ⏸ On Hold     | —       | Currently optional                         |
| 16 | Header extensibility (flags, future fields)      | ✅ Implemented | v3      | Reserved space available                   |
| 17 | Field name uniqueness validation                 | 🔜 Planned     | v4?     | May be implemented in generator/parser     |
| 18 | Nested structure support                         | ⏸ On Hold     | —       | Conflicts with linear simplicity           |
| 19 | SDK/Parsers: Python                              | ✅ Partial     | v3      | Basic parser available                     |
| 20 | SDK/Parsers: Go                                  | ✅ Partial     | v3      | Basic parser available                     |
| 21 | SDK/Parsers: PHP                                 | ✅ Partial     | v3      | Basic parser available                     |
| 22 | SDK/Parsers: JS (Node, Deno, Bun)                | ✅ Partial     | v3      | Basic parser available                     |
| 23 | SDK/Parsers: TS (Node, Deno, Bun)                | 🔜 Planned     | v3      | Libraries in development                   |
| 24 | SDK/Parsers: C/C++/Rust/C#                       | 🔜 Planned     | v3      | Awaiting demand confirmation               |

---

### 🗂 Legend

* ✅ **Implemented** - Fully implemented and in use
* 🔜 **Planned** - Scheduled for next version
* ⏸ **On Hold** - Deemed non-essential or postponed

---

## Message Structure
```
┌────────────────────────────────────────────────────────────────────────────────┐
│                               BSDMP Message Structure                          │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                            Complete BSDMP Message                        │  │
│  ├────────────────────┬─────────────────────────────────────────────────────┤  │
│  │   Magic (4B)       │                  Header (24B)                       │  │
│  │ "BSDM" signature   ├───────────────┬─────────────────┬───────────────────┤  │
│  │                    │ Version (1B)  │ Extension (1B)  │ Compression (1B)  │  │
│  │                    ├────────────┬──┴─────────────────┴─────┬─────────────┤  │
│  │                    │ Flags (8B) │   Data Block Size (4B)   │ Res (8B)    │  │   
│  ├────────────────────┴────────────┴──────────────────────────┴─────────────┤  │
│  │                                                                          │  │
│  │  ┌────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                  Compressed Data Block (variable)                  │  │  │
│  │  └────────────────────────────────────────────────────────────────────┘  │  │
│  │  ┌──────────────────┐                                                    │  │
│  │  │   CRC32 (4B)     │                                                    │  │
│  │  └──────────────────┘                                                    │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │               Expanded Data Block Structure (when decompressed)          │  │
│  ├──────────────────────┬─────────────────────┬─────────────────────────────┤  │
│  │   Frame Count (4B)   │   FSE Length (4B)   │   FSE String (variable)     │  │
│  ├────────────────────┬─┴─────────────────────┴─────┬───────────────────────┤  │
│  │   FEE Length (4B)  │    FEE String (variable)    │   Title Length (4B)   │  │
│  ├────────────────────┴─────────────────────────────┴───────────────────────┤  │
│  │                                                                          │  │
│  │  ┌────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                        Title Block (variable)                      │  │  │
│  │  ├──────────────────────────────┬─────────────────────────────────────┤  │  │
│  │  │  Field Name Length (2B)      │   Field Name (UTF-8) (variable)     │  │  │
│  │  ├────────────────────────────────────────────────────────────────────┤  │  │
│  │  │        Field Type (1B)            │       Type Length (1B)         │  │  │
│  │  └───────────────────────────────────┴────────────────────────────────┘  │  │
│  │                                                                          │  │
│  │  ┌────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                          Frame Array [N]                           │  │  │
│  │  ├────────────────────────────────────────────────────────────────────┤  │  │
│  │  │                                                                    │  │  │
│  │  │  ┌──────────────────────────────────────────────────────────────┐  │  │  │
│  │  │  │                           BSDMP Frame                        │  │  │  │
│  │  │  ├────────────────────┬─────────────────────┬───────────────────┤  │  │  │
│  │  │  │  FSE Marker (VL)   │  Frame Number (4B)  │ Field Count (2B)  │  │  │  │
│  │  │  ├────────────────────┴─────────────────────┴───────────────────┤  │  │  │
│  │  │  │                                                              │  │  │  │
│  │  │  │  ┌────────────────────────────────────────────────────────┐  │  │  │  │
│  │  │  │  │                    Field Data [N]                      │  │  │  │  │
│  │  │  │  ├──────────────────┬───────────────────┬─────────────────┤  │  │  │  │
│  │  │  │  │ Title Index (1B) │ Field Length (VB) │ Field Data (VL) │  │  │  │  │
│  │  │  │  └──────────────────┴───────────────────┴─────────────────┘  │  │  │  │
│  │  │  │                                                              │  │  │  │
│  │  │  │  FEE Marker (VL)                                             │  │  │  │
│  │  │  └──────────────────────────────────────────────────────────────┘  │  │  │
│  │  │                                                                    │  │  │
│  │  └────────────────────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────┘  

Legend:                                                                       
 - B = Bytes                                                                   
 - VB = Variable bytes (determined by `type_len` from corresponding Title Entry)  
 - VL = Variable length                                                        
 - FSE = Frame Start Marker                                                 
 - FEE = Frame End Marker                                                   
 - N = Number of elements (frames/fields)    
```

## Example Binary Data
[Example binary data](_docs/fullmessage.md)