import struct
import json
import struct
import json
import gzip
import zlib


class CompressionType:
    RAW = 0
    GZIP = 1
    ZLIB = 2

    @staticmethod
    def compress(data: bytes, ctype: int) -> bytes:
        if ctype == CompressionType.RAW:
            return data
        elif ctype == CompressionType.GZIP:
            return gzip.compress(data)
        elif ctype == CompressionType.ZLIB:
            return zlib.compress(data)
        else:
            raise BSDMPError(f"Unknown compression type {ctype}")

    @staticmethod
    def decompress(data: bytes, ctype: int) -> bytes:
        if ctype == CompressionType.RAW:
            return data
        elif ctype == CompressionType.GZIP:
            return gzip.decompress(data)
        elif ctype == CompressionType.ZLIB:
            return zlib.decompress(data)
        else:
            raise BSDMPError(f"Unknown compression type {ctype}")


class BSDMPError(Exception):
    pass


class FieldType:
    STRING = 0x01
    INT = 0x02
    FLOAT = 0x03
    BOOL = 0x04
    JSON = 0x05

    @staticmethod
    def detect(value):
        if isinstance(value, bool):
            return FieldType.BOOL
        elif isinstance(value, int):
            return FieldType.INT
        elif isinstance(value, float):
            return FieldType.FLOAT
        elif isinstance(value, (dict, list)):
            return FieldType.JSON
        return FieldType.STRING

    @staticmethod
    def encode(value, ftype):
        if ftype == FieldType.BOOL:
            return b"\x01" if value else b"\x00"
        elif ftype == FieldType.INT:
            return value.to_bytes(8, byteorder="little", signed=True)
        elif ftype == FieldType.FLOAT:
            return struct.pack("<d", value)
        elif ftype == FieldType.JSON:
            return json.dumps(value).encode()
        elif ftype == FieldType.STRING:
            return value.encode("utf-8") if isinstance(value, str) else value
        return value

    @staticmethod
    def decode(data, ftype):
        try:
            if ftype == FieldType.BOOL:
                return data != b"\x00"
            elif ftype == FieldType.INT:
                return int.from_bytes(data, byteorder="little", signed=True)
            elif ftype == FieldType.FLOAT:
                return struct.unpack("<d", data)[0]
            elif ftype == FieldType.JSON:
                return json.loads(data)
            elif ftype == FieldType.STRING:
                return data.decode("utf-8")
        except Exception:
            return data
        return data


class BSDMPHeader:
    FORMAT = "<I I I"  # Version, Compression, DataBlockSize

    def __init__(self, version=1, compression=0, data_block=b""):
        self.version = version
        self.compression = compression
        self._raw_data_block = data_block

    def encode(self) -> bytes:
        compressed_data = CompressionType.compress(
            self._raw_data_block, self.compression
        )
        return (
            struct.pack(
                self.FORMAT,
                self.version,
                self.compression,
                len(compressed_data),
            )
            + compressed_data
        )

    @classmethod
    def decode(cls, data: bytes):
        hdr_size = struct.calcsize(cls.FORMAT)
        if len(data) < hdr_size:
            raise BSDMPError("Header too short")

        version, compression, db_size = struct.unpack(cls.FORMAT, data[:hdr_size])
        compressed_block = data[hdr_size : hdr_size + db_size]
        raw_block = CompressionType.decompress(compressed_block, compression)
        return cls(version, compression, raw_block), hdr_size + db_size


class BSDMPDataBlock:
    def __init__(
        self,
        frame_count=0,
        fse=b"FRAME>",
        fee=b"<FRAME",
        title: list[tuple[str, int]] | None = None,
        frames=None,
    ):
        self.frame_count = frame_count
        self.fse = fse
        self.fee = fee
        self.title = title or []
        self.frames = frames or []

    def encode_title(self):
        data = b""
        for name, t in self.title:
            name_b = name.encode("utf-8")
            data += struct.pack("<H", len(name_b)) + name_b + struct.pack("<B", t)
        return data

    def encode(self):
        self.frame_count = len(self.frames)
        result = struct.pack("<I", self.frame_count)
        result += struct.pack("<I", len(self.fse)) + self.fse
        result += struct.pack("<I", len(self.fee)) + self.fee
        title_data = self.encode_title()
        result += struct.pack("<I", len(title_data)) + title_data
        for f in self.frames:
            result += f.encode(self.fse, self.fee)
        return result

    @classmethod
    def decode(cls, data: bytes):
        offset = 0
        frame_count = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4

        fse_len = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4
        fse = data[offset : offset + fse_len]
        offset += fse_len

        fee_len = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4
        fee = data[offset : offset + fee_len]
        offset += fee_len

        title_len = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4
        title_raw = data[offset : offset + title_len]
        offset += title_len

        title = []
        tpos = 0
        while tpos < len(title_raw):
            nlen = struct.unpack("<H", title_raw[tpos : tpos + 2])[0]
            tpos += 2
            name = title_raw[tpos : tpos + nlen].decode("utf-8")
            tpos += nlen
            tcode = title_raw[tpos]
            tpos += 1
            title.append((name, tcode))

        frames = []
        for _ in range(frame_count):
            frame, used = BSDMPFrame.decode(data[offset:], fse, fee)
            frames.append(frame)
            offset += used

        return cls(frame_count, fse, fee, title, frames)


class BSDMPFrame:
    def __init__(self, frame_num=1, fields=None):
        self.frame_num = frame_num
        self.fields = fields or []

    def encode(self, fse: bytes, fee: bytes) -> bytes:
        field_data = b""
        for val in self.fields:
            val = val if isinstance(val, bytes) else val.encode("utf-8")
            field_data += struct.pack("<H", len(val)) + val

        full_len = len(field_data) + len(fee)

        return (
            fse
            + struct.pack("<I", self.frame_num)
            + struct.pack("<I", full_len)
            + field_data
            + fee
        )

    @classmethod
    def decode(cls, data: bytes, fse: bytes, fee: bytes):
        if not data.startswith(fse):
            raise BSDMPError("Frame start not found")

        offset = len(fse)

        if offset + 8 > len(data):
            raise BSDMPError("Frame too short to contain header")

        frame_num = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4
        full_len = struct.unpack("<I", data[offset : offset + 4])[0]
        offset += 4

        if offset + full_len - len(fee) > len(data):
            raise BSDMPError("Frame full_len exceeds available data")

        field_end = offset + full_len - len(fee)
        fields = []

        while offset + 2 <= field_end:
            flen = struct.unpack("<H", data[offset : offset + 2])[0]
            offset += 2

            if offset + flen > field_end:
                raise BSDMPError("Field length exceeds frame boundary")

            field = data[offset : offset + flen]
            offset += flen
            fields.append(field)

        if data[offset : offset + len(fee)] != fee:
            raise BSDMPError("Frame end marker not found")

        offset += len(fee)
        return cls(frame_num, fields), offset


class BSDMPClient:
    def __init__(
        self,
        version=1,
        compression=CompressionType.RAW,
        fse=b"FRAME>",
        fee=b"<FRAME",
    ):
        self.version = version
        self.compression = compression
        self.fse = fse
        self.fee = fee
        self.frames = []
        self.title = []
        self.counter = 1

    def format(self, fields: list[str]):
        self.title = [(name, FieldType.STRING) for name in fields]

    def format_with_types(self, field_definitions: list[tuple[str, int]]):
        """Задает формат полей с указанием типов для каждого поля.

        Args:
            field_definitions: Список кортежей (имя_поля, тип_поля)

        Пример:
            format_with_types([
                ("name", FieldType.STRING),
                ("age", FieldType.INT),
                ("is_active", FieldType.BOOL)
            ])
        """
        valid_types = {
            FieldType.STRING,
            FieldType.INT,
            FieldType.FLOAT,
            FieldType.BOOL,
            FieldType.JSON,
        }

        for _, field_type in field_definitions:
            if field_type not in valid_types:
                raise ValueError(f"Недопустимый тип поля: {field_type}")

        self.title = field_definitions.copy()

    def frame(self, values: list):
        encoded_fields = [FieldType.encode(v, FieldType.detect(v)) for v in values]
        self.frames.append(BSDMPFrame(self.counter, encoded_fields))
        self.counter += 1

    def encode(self) -> bytes:
        block = BSDMPDataBlock(
            frame_count=len(self.frames),
            fse=self.fse,
            fee=self.fee,
            title=self.title,
            frames=self.frames,
        ).encode()
        header = BSDMPHeader(
            version=self.version,
            compression=self.compression,
            data_block=block,
        )
        return header.encode()


class BSDMPServer:
    def __init__(self):
        self.title = []
        self.frames = []

    def decode(self, raw: bytes):
        header, _ = BSDMPHeader.decode(raw)
        block = BSDMPDataBlock.decode(header._raw_data_block)
        self.title = block.title
        self.frames = []

        for frame in block.frames:
            mapped = {}
            for i, field in enumerate(frame.fields):
                if i < len(self.title):
                    name, ftype = self.title[i]
                    mapped[name] = FieldType.decode(field, ftype)
                else:
                    mapped[str(i)] = field
            self.frames.append(mapped)
