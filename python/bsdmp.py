import bz2
import gzip
import json
import lzma
import struct
import zlib
from typing import Any, Dict, List, Optional, Tuple, Union
from enum import IntEnum


class BSDMPFieldType(IntEnum):
    STRING = 1
    INT = 2
    FLOAT = 3
    BOOL = 4
    JSON = 5
    BINARY = 6


class BSDMPFieldSize(IntEnum):
    B255 = 1
    K64 = 2
    G4 = 4


class BSDMPCompressor:
    _compressors = {}

    @classmethod
    def register(cls, method: int, compress_fn, decompress_fn):
        cls._compressors[method] = (compress_fn, decompress_fn)

    @classmethod
    def compress(cls, method: int, data: bytes) -> bytes:
        compress_fn = cls._compressors.get(method, (lambda x: x,))[0]
        return compress_fn(data)

    @classmethod
    def decompress(cls, method: int, data: bytes) -> bytes:
        decompress_fn = cls._compressors.get(method, (None, lambda x: x))[1]
        return decompress_fn(data)


class BSDMPCRCDriver:
    @staticmethod
    def calculate(data: bytes) -> int:
        """Рассчитывает CRC (по умолчанию zlib.crc32)."""
        # print(data.hex(), zlib.crc32(data))
        return zlib.crc32(data) & 0xFFFFFFFF

    @staticmethod
    def check(data: bytes, expected_crc: int) -> bool:
        # print(expected_crc, BSDMPCRCDriver.calculate(data))
        """Проверяет совпадение CRC."""
        return BSDMPCRCDriver.calculate(data) == expected_crc


class BSDMPTitleEntry:
    def __init__(self, name: str, type_code: int, type_len: int):
        if type_code not in BSDMPFieldType.__members__.values():
            raise ValueError(f"Invalid type_code: {type_code}")
        if type_len not in BSDMPFieldSize.__members__.values():
            raise ValueError(f"Invalid type_len: {type_len}")
        self.name = name
        self.type_code = type_code
        self.type_len = type_len

    def to_bytes(self) -> bytes:
        name_bytes = self.name.encode("utf-8")
        return (
            struct.pack("<H", len(name_bytes))
            + name_bytes
            + struct.pack("BB", self.type_code, self.type_len)
        )

    @classmethod
    def from_bytes(cls, data: bytes, offset: int) -> Tuple["BSDMPTitleEntry", int]:
        if offset + 2 > len(data):
            raise ValueError("Insufficient data for name_len")
        name_len = struct.unpack_from("<H", data, offset)[0]
        offset += 2
        if offset + name_len > len(data):
            raise ValueError("Insufficient data for name")
        name = data[offset : offset + name_len].decode("utf-8")
        offset += name_len
        if offset + 2 > len(data):
            raise ValueError("Insufficient data for type_code and type_len")
        type_code, type_len = struct.unpack_from("BB", data, offset)
        offset += 2
        return cls(name, type_code, type_len), offset


class BSDMPField:
    def __init__(self, title_index: int, data: bytes, type_len: int):
        self.title_index = title_index
        self.data = data
        self.type_len = type_len  # Кол-во байт для длины поля

    def to_bytes(self) -> bytes:
        length = len(self.data)
        if self.type_len == BSDMPFieldSize.B255 and length > 255:
            raise ValueError("Field length exceeds 255 bytes for type_len=BSDMPFieldSize.B255")
        elif self.type_len == BSDMPFieldSize.K64 and length > 65535:
            raise ValueError("Field length exceeds 65535 bytes for type_len=BSDMPFieldSize.K64")
        elif self.type_len == BSDMPFieldSize.G4 and length > 0xFFFFFFFF:
            raise ValueError("Field length exceeds 4GB for type_len=BSDMPFieldSize.G4")
        if self.type_len == 1:
            length_bytes = struct.pack("<B", length)
        elif self.type_len == 2:
            length_bytes = struct.pack("<H", length)
        elif self.type_len == 4:
            length_bytes = struct.pack("<I", length)
        else:
            raise ValueError(f"Unsupported type_len {self.type_len}")
        return struct.pack("<B", self.title_index) + length_bytes + self.data

    @classmethod
    def from_bytes(
        cls, data: bytes, offset: int, type_len: int
    ) -> Tuple["BSDMPField", int]:
        title_index = data[offset]
        offset += 1
        if type_len == 1:
            field_len = struct.unpack_from("<B", data, offset)[0]
        elif type_len == 2:
            field_len = struct.unpack_from("<H", data, offset)[0]
        elif type_len == 4:
            field_len = struct.unpack_from("<I", data, offset)[0]
        else:
            raise ValueError(f"Unsupported type_len {type_len}")
        offset += type_len
        field_data = data[offset : offset + field_len]
        offset += field_len
        return cls(title_index, field_data, type_len), offset


class BSDMPFrame:
    def __init__(
        self, fse: bytes, frame_num: int, fields: List[BSDMPField], fee: bytes
    ):
        self.fse = fse
        self.frame_num = frame_num
        self.fields = fields
        self.fee = fee

    def to_bytes(self) -> bytes:
        b = bytearray()
        b.extend(self.fse)
        b.extend(struct.pack("<I", self.frame_num))
        b.extend(struct.pack("<H", len(self.fields)))
        for field in self.fields:
            b.extend(field.to_bytes())
        b.extend(self.fee)
        return bytes(b)

    @classmethod
    def from_bytes(
        cls,
        data: bytes,
        offset: int,
        fse: bytes,
        fee: bytes,
        title_entries: List[BSDMPTitleEntry],
    ) -> Tuple["BSDMPFrame", int]:
        # Проверяем маркер начала
        if data[offset : offset + len(fse)] != fse:
            raise ValueError("Frame start marker mismatch")
        offset += len(fse)
        frame_num = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        field_count = struct.unpack_from("<H", data, offset)[0]
        offset += 2
        fields = []
        for _ in range(field_count):
            # Получаем type_len из title_entries по title_index (читаем title_index первым)
            title_index = data[offset]
            type_len = title_entries[title_index].type_len
            field, offset = BSDMPField.from_bytes(data, offset, type_len)
            fields.append(field)
        # Проверяем маркер конца
        if data[offset : offset + len(fee)] != fee:
            raise ValueError("Frame end marker mismatch")
        offset += len(fee)
        return cls(fse, frame_num, fields, fee), offset


class BSDMPDataBlock:
    def __init__(
        self,
        frame_count: int,
        fse: bytes,
        fee: bytes,
        title_entries: List[BSDMPTitleEntry],
        frames: List[BSDMPFrame],
    ):
        self.frame_count = frame_count
        self.fse = fse
        self.fee = fee
        self.title_entries = title_entries
        self.frames = frames

    def to_bytes(self) -> bytes:
        b = bytearray()
        b.extend(struct.pack("<I", self.frame_count))
        b.extend(struct.pack("<I", len(self.fse)))
        b.extend(self.fse)
        b.extend(struct.pack("<I", len(self.fee)))
        b.extend(self.fee)
        # Сериализуем title entries
        title_bytes = b"".join(te.to_bytes() for te in self.title_entries)
        b.extend(struct.pack("<I", len(title_bytes)))
        b.extend(title_bytes)
        # Сериализуем frames
        for frame in self.frames:
            b.extend(frame.to_bytes())
        return bytes(b)

    @classmethod
    def from_bytes(cls, data: bytes, offset: int) -> Tuple["BSDMPDataBlock", int]:
        if offset + 4 > len(data):
            raise ValueError("Insufficient data for frame_count")
        frame_count = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        if offset + 4 > len(data):
            raise ValueError("Insufficient data for fse_len")
        fse_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        if offset + fse_len > len(data):
            raise ValueError("Insufficient data for fse")
        fse = data[offset : offset + fse_len]
        offset += fse_len
        fee_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        fee = data[offset : offset + fee_len]
        offset += fee_len
        title_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        title_data = data[offset : offset + title_len]
        offset += title_len

        # Распарсим title entries
        title_entries = []
        title_offset = 0
        while title_offset < len(title_data):
            te, title_offset = BSDMPTitleEntry.from_bytes(title_data, title_offset)
            title_entries.append(te)

        # Распарсим frames
        frames = []
        for _ in range(frame_count):
            frame, offset = BSDMPFrame.from_bytes(data, offset, fse, fee, title_entries)
            frames.append(frame)

        return cls(frame_count, fse, fee, title_entries, frames), offset


class BSDMPHeader:
    def __init__(
        self,
        version: int,
        extension: int,
        compression: int,
        data_block_size: int,
        binary_flags: Optional[bytes] = None,
    ):
        self.version = version
        self.extension = extension
        self.compression = compression
        self.reserved1 = 0  # как uint8_t
        self.binary_flags = bytearray(binary_flags or b"\x00" * 8)
        self.data_block_size = data_block_size
        self.reserved2 = b"\x00" * 8

    def to_bytes(self) -> bytes:
        return struct.pack(
            "<BBBB8sI8s",
            self.version,
            self.extension,
            self.compression,
            self.reserved1,
            bytes(self.binary_flags),
            self.data_block_size,
            self.reserved2,
        )

    @classmethod
    def from_bytes(cls, data: bytes, offset: int = 0) -> Tuple["BSDMPHeader", int]:
        (
            version,
            extension,
            compression,
            reserved1,
            binary_flags,
            data_block_size,
            reserved2,
        ) = struct.unpack_from("<BBBB8sI8s", data, offset)
        offset += 1 + 1 + 1 + 1 + 8 + 4 + 8  # 24 байта
        obj = cls(version, extension, compression, data_block_size, binary_flags)
        obj.reserved1 = reserved1
        obj.reserved2 = reserved2
        return obj, offset

    def set_flag(self, bit_index: int, value: bool):
        if not 0 <= bit_index < 64:
            raise ValueError("bit_index must be in 0..63")
        byte_index = bit_index // 8
        bit_in_byte = bit_index % 8
        if value:
            self.binary_flags[byte_index] |= 1 << bit_in_byte
        else:
            self.binary_flags[byte_index] &= ~(1 << bit_in_byte)

    def get_flag(self, bit_index: int) -> bool:
        if not 0 <= bit_index < 64:
            raise ValueError("bit_index must be in 0..63")
        byte_index = bit_index // 8
        bit_in_byte = bit_index % 8
        return (self.binary_flags[byte_index] >> bit_in_byte) & 1 == 1


class BSDMPMessage:
    MAGIC = b"BSDM"

    def __init__(self, header: BSDMPHeader, data_block: BSDMPDataBlock, crc: int):
        self.header = header
        self.data_block = data_block
        self.crc = crc

    def to_bytes(self) -> bytes:
        raw_data = self.data_block.to_bytes()
        compressed_data = BSDMPCompressor.compress(self.header.compression, raw_data)

        self.header.data_block_size = len(compressed_data)
        header_bytes = self.header.to_bytes()

        # Используем драйвер CRC
        self.crc = BSDMPCRCDriver.calculate(compressed_data)

        return self.MAGIC + header_bytes + compressed_data + struct.pack("<I", self.crc)

    @classmethod
    def from_bytes(cls, data: bytes) -> "BSDMPMessage":
        if data[:4] != cls.MAGIC:
            raise ValueError("Invalid BSDMP magic")

        header, offset = BSDMPHeader.from_bytes(data, 4)

        if offset + header.data_block_size + 4 > len(data):
            raise ValueError("Data too short for declared data_block_size and CRC")

        compressed_data = data[offset : offset + header.data_block_size]
        offset += header.data_block_size

        (crc_val,) = struct.unpack_from("<I", data, offset)

        # декомпрессия
        raw_data = BSDMPCompressor.decompress(header.compression, compressed_data)

        calc_crc = zlib.crc32(compressed_data) & 0xFFFFFFFF
        if calc_crc != crc_val:
            raise ValueError("CRC mismatch")

        data_block, _ = BSDMPDataBlock.from_bytes(raw_data, 0)
        return cls(header, data_block, crc_val)


class BSDMPPack:
    def __init__(
        self,
        compression: int = 0,
        extension: int = 0,
        binary_flags: Optional[bytes] = None,
        fse: bytes = b"\xfa\xfb\xfc",
        fee: bytes = b"\xfd\xfe\xff",
    ):
        self._title_entries: List[BSDMPTitleEntry] = []
        self._frames_data: List[List[Any]] = []
        self._compression = compression
        self._extension = extension
        if binary_flags is not None:
            if len(binary_flags) != 8:
                raise ValueError("binary_flags должен быть длиной 8 байт")
            self._binary_flags = bytearray(binary_flags)
        else:
            self._binary_flags = bytearray(b"\x00" * 8)
        self._fse = fse
        self._fee = fee

    def title(self, fields: List[Tuple[str, int, int]]):
        names = [name for name, _, _ in fields]
        if len(names) != len(set(names)):
            raise ValueError("Field names must be unique")
        self._title_entries = [
            BSDMPTitleEntry(name, type_code, type_len)
            for name, type_code, type_len in fields
        ]

    def frame(self, values: Union[List[Any], Dict[str, Optional[Any]]]):
        if isinstance(values, dict):
            ordered_values = []
            for te in self._title_entries:
                ordered_values.append(values.get(te.name, None))
            values = ordered_values
        else:
            if len(values) != len(self._title_entries):
                raise ValueError(
                    "Количество значений не совпадает с количеством заголовков"
                )

        self._frames_data.append(values)

    def pack(self) -> bytes:
        frames = []

        for i, frame_values in enumerate(self._frames_data, start=1):
            fields = []
            for idx, value in enumerate(frame_values):
                if value is None:
                    continue
                title_entry = self._title_entries[idx]
                data_bytes = self._encode_value(value, title_entry.type_code)
                fields.append(
                    BSDMPField(
                        title_index=idx,
                        type_len=title_entry.type_len,
                        data=data_bytes,
                    )
                )
            frame = BSDMPFrame(self._fse, i, fields, self._fee)
            frames.append(frame)

        data_block = BSDMPDataBlock(
            frame_count=len(frames),
            fse=self._fse,
            fee=self._fee,
            title_entries=self._title_entries,
            frames=frames,
        )

        raw_data = data_block.to_bytes()

        header = BSDMPHeader(
            version=3,
            extension=self._extension,
            compression=self._compression,
            data_block_size=len(raw_data),
            binary_flags=self._binary_flags,
        )

        crc_val = BSDMPCRCDriver.calculate(raw_data)
        message = BSDMPMessage(header, data_block, crc_val)
        return message.to_bytes()

    def _encode_value(self, value, type_code) -> bytes:
        if type_code == BSDMPFieldType.STRING:
            if isinstance(value, str):
                return value.encode("utf-8")
            elif isinstance(value, bytes):
                return value
            else:
                return str(value).encode("utf-8")
        elif type_code == BSDMPFieldType.INT:
            return struct.pack("<q", int(value))
        elif type_code == BSDMPFieldType.FLOAT:
            return struct.pack("<d", value)
        elif type_code == BSDMPFieldType.BOOL:
            return b"\x01" if value else b"\x00"
        elif type_code == BSDMPFieldType.JSON:
            return json.dumps(value).encode("utf-8")
        elif type_code == BSDMPFieldType.BINARY:
            return value
        else:
            raise NotImplementedError(
                f"Encoding for type_code {type_code} not implemented"
            )

    def set_flag(self, bit_index: int, value: bool):
        if not 0 <= bit_index < 64:
            raise ValueError("bit_index must be in 0..63")
        byte_index = bit_index // 8
        bit_in_byte = bit_index % 8
        if value:
            self._binary_flags[byte_index] |= 1 << bit_in_byte
        else:
            self._binary_flags[byte_index] &= ~(1 << bit_in_byte)


class BSDMPUnpack:
    def __init__(self, data: bytes, strict: bool = False):
        self._message = BSDMPMessage.from_bytes(data)
        self.title_entries = self._message.data_block.title_entries
        self.strict = strict
        self.frames = self._parse_frames()
        self._binary_flags = self._message.header.binary_flags

    def get_flag(self, bit_index: int) -> bool:
        if not 0 <= bit_index < 64:
            raise ValueError("bit_index must be in 0..63")
        byte_index = bit_index // 8
        bit_in_byte = bit_index % 8
        return (self._binary_flags[byte_index] >> bit_in_byte) & 1 == 1

    def _parse_frames(self):
        parsed_frames = []
        for frame in self._message.data_block.frames:
            if self.strict:
                frame_dict: dict[str, Any] = {
                    te.name: None for te in self.title_entries
                }
                for field in frame.fields:
                    title_entry = self.title_entries[field.title_index]
                    val = self._decode_value(field.data, title_entry.type_code)
                    frame_dict[title_entry.name] = val
            else:
                frame_dict = {}
                for field in frame.fields:
                    title_entry = self.title_entries[field.title_index]
                    val = self._decode_value(field.data, title_entry.type_code)
                    frame_dict[title_entry.name] = val
            parsed_frames.append(frame_dict)
        return parsed_frames

    def _decode_value(self, data: bytes, type_code):
        if type_code == BSDMPFieldType.STRING:
            return data.decode("utf-8")
        elif type_code == BSDMPFieldType.INT:
            return struct.unpack("<q", data)[0]
        elif type_code == BSDMPFieldType.FLOAT:
            return struct.unpack("<d", data)[0]
        elif type_code == BSDMPFieldType.BOOL:
            return bool(data[0])
        elif type_code == BSDMPFieldType.JSON:
            return json.loads(data.decode("utf-8"))
        elif type_code == BSDMPFieldType.BINARY:
            return data
        else:
            raise NotImplementedError(
                f"Decoding for type_code {type_code} not implemented"
            )


# Метод 0 — без сжатия
BSDMPCompressor.register(0, lambda x: x, lambda x: x)
# Метод 1 — zlib
BSDMPCompressor.register(1, zlib.compress, zlib.decompress)
# Метод 2 — gzip
BSDMPCompressor.register(
    2, lambda x: gzip.compress(x, compresslevel=9), lambda x: gzip.decompress(x)
)
# Метод 3 — bzip2
BSDMPCompressor.register(
    3, lambda x: bz2.compress(x, compresslevel=9), lambda x: bz2.decompress(x)
)
# Метод 4 — lzma
BSDMPCompressor.register(
    4, lambda x: lzma.compress(x, preset=9), lambda x: lzma.decompress(x)
)
