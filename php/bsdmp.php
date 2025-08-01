<?php

declare(strict_types=1);

namespace BSDMP;

use InvalidArgumentException;
use RuntimeException;
use ValueError;

// Перечисления
enum FieldType: int
{
    case STRING = 1;
    case INT = 2;
    case FLOAT = 3;
    case BOOL = 4;
    case JSON = 5;
    case BINARY = 6;
}

enum FieldSize: int
{
    case B255 = 1;
    case K64 = 2;
    case G4 = 4;
}

enum CompressionMethod: int
{
    case NONE = 0;
    case ZLIB = 1;
    case GZIP = 2;
    case BZIP2 = 3;
    case LZMA = 4;
}

// Интерфейс компрессора
interface Compressor
{
    public function compress(string $data): string;
    public function decompress(string $data): string;
}

class NoneCompressor implements Compressor
{
    public function compress(string $data): string
    {
        return $data;
    }
    public function decompress(string $data): string
    {
        return $data;
    }
}

class ZlibCompressor implements Compressor
{
    public function compress(string $data): string
    {
        $compressed = zlib_encode($data, ZLIB_ENCODING_DEFLATE, 9);
        if ($compressed === false) {
            throw new RuntimeException('ZLIB compression failed');
        }
        return $compressed;
    }

    public function decompress(string $data): string
    {
        $decompressed = zlib_decode($data);
        if ($decompressed === false) {
            throw new RuntimeException('ZLIB decompression failed');
        }
        return $decompressed;
    }
}

class GzipCompressor implements Compressor
{
    public function compress(string $data): string
    {
        $compressed = gzcompress($data, 9);
        if ($compressed === false) {
            throw new RuntimeException('GZIP compression failed');
        }
        return $compressed;
    }

    public function decompress(string $data): string
    {
        $decompressed = gzuncompress($data);
        if ($decompressed === false) {
            throw new RuntimeException('GZIP decompression failed');
        }
        return $decompressed;
    }
}

class Bzip2Compressor implements Compressor
{
    public function compress(string $data): string
    {
        $compressed = bzcompress($data, 9);
        if (is_int($compressed)) {
            throw new RuntimeException('BZIP2 compression failed');
        }
        return $compressed;
    }

    public function decompress(string $data): string
    {
        $decompressed = bzdecompress($data);
        if (is_int($decompressed)) {
            throw new RuntimeException('BZIP2 decompression failed');
        }
        return $decompressed;
    }
}

class LzmaCompressor implements Compressor
{
    public function compress(string $data): string
    {
        // LZMA не встроен в PHP, требуется расширение или внешняя библиотека
        throw new RuntimeException('LZMA compression not supported in PHP');
    }

    public function decompress(string $data): string
    {
        throw new RuntimeException('LZMA decompression not supported in PHP');
    }
}

// Реестр компрессоров
class CompressorRegistry
{
    private static array $compressors = [
        CompressionMethod::NONE->value => NoneCompressor::class,
        CompressionMethod::ZLIB->value => ZlibCompressor::class,
        CompressionMethod::GZIP->value => GzipCompressor::class,
        CompressionMethod::BZIP2->value => Bzip2Compressor::class,
        CompressionMethod::LZMA->value => LzmaCompressor::class,
    ];

    public static function get(CompressionMethod $method): Compressor
    {
        $class = self::$compressors[$method->value] ?? throw new InvalidArgumentException("Unsupported compression method: {$method->value}");
        return new $class();
    }
}

// CRC32 драйвер
class CRCDriver
{
    public static function calculate(string $data): int
    {
        return crc32($data);
    }

    public static function check(string $data, int $expected): bool
    {
        return (self::calculate($data) & 0xFFFFFFFF) === ($expected & 0xFFFFFFFF);
    }
}

// TitleEntry
class TitleEntry
{
    public function __construct(
        public string $name,
        public FieldType $typeCode,
        public FieldSize $typeLen
    ) {
        if (strlen($name) > 65535) {
            throw new ValueError('Field name too long');
        }
        if (!in_array($typeLen->value, array_column(FieldSize::cases(), 'value'))) {
            throw new ValueError('Invalid typeLen');
        }
    }

    public function toBytes(): string
    {
        $nameBytes = mb_convert_encoding($this->name, 'UTF-8');
        return pack('v', strlen($nameBytes)) . $nameBytes . pack('CC', $this->typeCode->value, $this->typeLen->value);
    }

    public static function fromBytes(string $data, int &$offset): self
    {
        if ($offset + 2 > strlen($data)) {
            throw new RuntimeException('Insufficient data for name_len');
        }
        $nameLen = unpack('v', substr($data, $offset, 2))[1];
        $offset += 2;
        if ($offset + $nameLen + 2 > strlen($data)) {
            throw new RuntimeException('Insufficient data for title entry');
        }
        $name = substr($data, $offset, $nameLen);
        $offset += $nameLen;
        [$typeCode, $typeLen] = array_values(unpack('C2', substr($data, $offset, 2)));
        $offset += 2;
        return new self(
            $name,
            FieldType::from($typeCode),
            FieldSize::from($typeLen)
        );
    }
}

// Field
class Field
{
    public function __construct(
        public int $titleIndex,
        public string $data,
        public FieldSize $typeLen
    ) {}

    public function toBytes(): string
    {
        $length = strlen($this->data);
        if ($this->typeLen === FieldSize::B255 && $length > 255) {
            throw new ValueError('Field length exceeds 255 bytes');
        } elseif ($this->typeLen === FieldSize::K64 && $length > 65535) {
            throw new ValueError('Field length exceeds 65535 bytes');
        } elseif ($this->typeLen === FieldSize::G4 && $length > 0xFFFFFFFF) {
            throw new ValueError('Field length exceeds 4GB');
        }

        $lengthBytes = match ($this->typeLen) {
            FieldSize::B255 => pack('C', $length),
            FieldSize::K64 => pack('v', $length),
            FieldSize::G4 => pack('V', $length),
        };
        return pack('C', $this->titleIndex) . $lengthBytes . $this->data;
    }

    public static function fromBytes(string $data, int &$offset, FieldSize $typeLen): self
    {
        if ($offset + 1 > strlen($data)) {
            throw new RuntimeException('Insufficient data for title_index');
        }
        $titleIndex = ord($data[$offset]);
        $offset += 1;

        $fieldLen = match ($typeLen) {
            FieldSize::B255 => unpack('C', substr($data, $offset, 1))[1],
            FieldSize::K64 => unpack('v', substr($data, $offset, 2))[1],
            FieldSize::G4 => unpack('V', substr($data, $offset, 4))[1],
        };
        $offset += $typeLen->value;

        if ($offset + $fieldLen > strlen($data)) {
            throw new RuntimeException('Insufficient data for field data');
        }
        $fieldData = substr($data, $offset, $fieldLen);
        $offset += $fieldLen;

        return new self($titleIndex, $fieldData, $typeLen);
    }
}

// Frame
class Frame
{
    public function __construct(
        public string $fse,
        public int $frameNum,
        public array $fields,
        public string $fee
    ) {}

    public function toBytes(): string
    {
        $result = $this->fse . pack('Vv', $this->frameNum, count($this->fields));
        foreach ($this->fields as $field) {
            $result .= $field->toBytes();
        }
        return $result . $this->fee;
    }

    public static function fromBytes(string $data, int &$offset, string $fse, string $fee, array $titleEntries): self
    {
        if (substr($data, $offset, strlen($fse)) !== $fse) {
            throw new RuntimeException('Frame start marker mismatch');
        }
        $offset += strlen($fse);

        if ($offset + 6 > strlen($data)) {
            throw new RuntimeException('Insufficient data for frame header');
        }
        [$frameNum, $fieldCount] = array_values(unpack('V1frameNum/v1fieldCount', substr($data, $offset, 6)));
        $offset += 6;

        $fields = [];
        for ($i = 0; $i < $fieldCount; $i++) {
            $titleIndex = ord($data[$offset]);
            if (!isset($titleEntries[$titleIndex])) {
                throw new RuntimeException('Invalid title index');
            }
            $fields[] = Field::fromBytes($data, $offset, $titleEntries[$titleIndex]->typeLen);
        }

        if (substr($data, $offset, strlen($fee)) !== $fee) {
            throw new RuntimeException('Frame end marker mismatch');
        }
        $offset += strlen($fee);

        return new self($fse, $frameNum, $fields, $fee);
    }
}

// DataBlock
class DataBlock
{
    public function __construct(
        public int $frameCount,
        public string $fse,
        public string $fee,
        public array $titleEntries,
        public array $frames
    ) {}

    public function toBytes(): string
    {
        $result = pack('V', $this->frameCount);
        $result .= pack('V', strlen($this->fse)) . $this->fse;
        $result .= pack('V', strlen($this->fee)) . $this->fee;

        $titleBytes = '';
        foreach ($this->titleEntries as $entry) {
            $titleBytes .= $entry->toBytes();
        }
        $result .= pack('V', strlen($titleBytes)) . $titleBytes;

        foreach ($this->frames as $frame) {
            $result .= $frame->toBytes();
        }

        return $result;
    }

    public static function fromBytes(string $data, int &$offset): self
    {
        if ($offset + 16 > strlen($data)) {
            throw new RuntimeException('Insufficient data for data block');
        }
        $frameCount = unpack('V', substr($data, $offset, 4))[1];
        $fseLen = unpack('V', substr($data, $offset + 4, 4))[1];
        $offset += 8;

        if ($offset + $fseLen > strlen($data)) {
            throw new RuntimeException('Insufficient data for FSE');
        }
        $fse = substr($data, $offset, $fseLen);
        $offset += $fseLen;

        $feeLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        if ($offset + $feeLen > strlen($data)) {
            throw new RuntimeException('Insufficient data for FEE');
        }
        $fee = substr($data, $offset, $feeLen);
        $offset += $feeLen;

        $titleLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        if ($offset + $titleLen > strlen($data)) {
            throw new RuntimeException('Insufficient data for title');
        }
        $titleData = substr($data, $offset, $titleLen);
        $offset += $titleLen;

        $titleEntries = [];
        $titleOffset = 0;
        while ($titleOffset < strlen($titleData)) {
            $entry = TitleEntry::fromBytes($titleData, $titleOffset);
            $titleEntries[] = $entry;
        }

        $frames = [];
        for ($i = 0; $i < $frameCount; $i++) {
            $frames[] = Frame::fromBytes($data, $offset, $fse, $fee, $titleEntries);
        }

        return new self($frameCount, $fse, $fee, $titleEntries, $frames);
    }
}

// Header
class Header
{
    public function __construct(
        public int $version,
        public int $extension,
        public CompressionMethod $compression,
        public int $dataBlockSize,
        public string $binaryFlags = "\0\0\0\0\0\0\0\0",
        public int $reserved1 = 0,
        public string $reserved2 = "\0\0\0\0\0\0\0\0"
    ) {
        if (strlen($binaryFlags) !== 8 || strlen($reserved2) !== 8) {
            throw new ValueError('Invalid flags or reserved length');
        }
    }

    public function toBytes(): string
    {
        return pack(
            'CCCCa8Va8',
            $this->version,
            $this->extension,
            $this->compression->value,
            $this->reserved1,
            $this->binaryFlags,
            $this->dataBlockSize,
            $this->reserved2
        );
    }

    public static function fromBytes(string $data, int &$offset): self
    {
        if ($offset + 24 > strlen($data)) {
            throw new RuntimeException('Insufficient data for header');
        }
        [
            'version' => $version,
            'extension' => $extension,
            'compression' => $compression,
            'reserved1' => $reserved1,
            'binaryFlags' => $binaryFlags,
            'dataBlockSize' => $dataBlockSize,
            'reserved2' => $reserved2
        ] = unpack('Cversion/Cextension/Ccompression/Creserved1/a8binaryFlags/VdataBlockSize/a8reserved2', substr($data, $offset, 24));
        $offset += 24;

        return new self(
            $version,
            $extension,
            CompressionMethod::from($compression),
            $dataBlockSize,
            $binaryFlags,
            $reserved1,
            $reserved2
        );
    }

    public function setFlag(int $bitIndex, bool $value): void
    {
        if ($bitIndex < 0 || $bitIndex >= 64) {
            throw new ValueError('bit_index must be in 0..63');
        }
        $byteIndex = intdiv($bitIndex, 8);
        $bitInByte = $bitIndex % 8;
        $bytes = str_split($this->binaryFlags);
        $byte = ord($bytes[$byteIndex]);
        $bytes[$byteIndex] = chr($value ? ($byte | (1 << $bitInByte)) : ($byte & ~(1 << $bitInByte)));
        $this->binaryFlags = implode('', $bytes);
    }
}

// Message
class Message
{
    private const MAGIC = 'BSDM';

    public function __construct(
        public Header $header,
        public DataBlock $dataBlock,
        public int $crc
    ) {}

    public function toBytes(): string
    {
        $rawData = $this->dataBlock->toBytes();
        $compressedData = CompressorRegistry::get($this->header->compression)->compress($rawData);
        $this->header->dataBlockSize = strlen($compressedData);
        $this->crc = CRCDriver::calculate($compressedData);

        return self::MAGIC . $this->header->toBytes() . $compressedData . pack('V', $this->crc);
    }

    public static function fromBytes(string $data): self
    {
        if (substr($data, 0, 4) !== self::MAGIC) {
            throw new RuntimeException('Invalid BSDMP magic');
        }
        $offset = 4;
        $header = Header::fromBytes($data, $offset);

        if ($offset + $header->dataBlockSize + 4 > strlen($data)) {
            throw new RuntimeException('Data too short for data block and CRC');
        }
        $compressedData = substr($data, $offset, $header->dataBlockSize);
        $offset += $header->dataBlockSize;
        $crc = unpack('V', substr($data, $offset, 4))[1];

        if (!CRCDriver::check($compressedData, $crc)) {
            throw new RuntimeException('CRC mismatch');
        }

        $rawData = CompressorRegistry::get($header->compression)->decompress($compressedData);
        $dataOffset = 0;
        $dataBlock = DataBlock::fromBytes($rawData, $dataOffset);

        return new self($header, $dataBlock, $crc);
    }
}

// Pack
class Pack
{
    private array $titleEntries = [];
    private array $framesData = [];
    private string $binaryFlags;

    public function __construct(
        private CompressionMethod $compression = CompressionMethod::NONE,
        private int $extension = 0,
        ?string $binaryFlags = null,
        private string $fse = "\xFA\xFB\xFC",
        private string $fee = "\xFD\xFE\xFF"
    ) {
        $this->binaryFlags = $binaryFlags ?? str_repeat("\0", 8);
        if (strlen($this->binaryFlags) !== 8) {
            throw new ValueError('binary_flags must be 8 bytes');
        }
    }

    public function title(array $fields): void
    {
        $names = array_column($fields, 0);
        if (count($names) !== count(array_unique($names))) {
            throw new ValueError('Field names must be unique');
        }
        $this->titleEntries = array_map(
            fn($f) => new TitleEntry($f[0], FieldType::from($f[1]), FieldSize::from($f[2])),
            $fields
        );
    }

    public function frame(array $values): void
    {
        if (array_is_list($values)) {
            if (count($values) !== count($this->titleEntries)) {
                throw new ValueError('Values count mismatch with title entries');
            }
            $this->framesData[] = $values;
        } else {
            $orderedValues = array_fill(0, count($this->titleEntries), null);
            foreach ($this->titleEntries as $i => $entry) {
                $orderedValues[$i] = $values[$entry->name] ?? null;
            }
            $this->framesData[] = $orderedValues;
        }
    }

    public function pack(): string
    {
        $frames = [];
        foreach ($this->framesData as $i => $frameValues) {
            $fields = [];
            foreach ($frameValues as $idx => $value) {
                if ($value === null) {
                    continue;
                }
                $titleEntry = $this->titleEntries[$idx];
                $dataBytes = $this->encodeValue($value, $titleEntry->typeCode);
                $fields[] = new Field($idx, $dataBytes, $titleEntry->typeLen);
            }
            $frames[] = new Frame($this->fse, $i + 1, $fields, $this->fee);
        }

        $dataBlock = new DataBlock(
            count($frames),
            $this->fse,
            $this->fee,
            $this->titleEntries,
            $frames
        );

        $header = new Header(
            version: 3,
            extension: $this->extension,
            compression: $this->compression,
            dataBlockSize: 0,
            binaryFlags: $this->binaryFlags
        );

        $message = new Message($header, $dataBlock, 0);
        return $message->toBytes();
    }

    private function encodeValue(mixed $value, FieldType $typeCode): string
    {
        return match ($typeCode) {
            FieldType::STRING => is_string($value) ? $value : (string)$value,
            FieldType::INT => pack('q', (int)$value),
            FieldType::FLOAT => pack('d', (float)$value),
            FieldType::BOOL => $value ? "\x01" : "\x00",
            FieldType::JSON => json_encode($value, JSON_THROW_ON_ERROR),
            FieldType::BINARY => $value,
            default => throw new RuntimeException("Unsupported type_code: {$typeCode->value}")
        };
    }

    public function setFlag(int $bitIndex, bool $value): void
    {
        if ($bitIndex < 0 || $bitIndex >= 64) {
            throw new ValueError('bit_index must be in 0..63');
        }
        $byteIndex = intdiv($bitIndex, 8);
        $bitInByte = $bitIndex % 8;
        $bytes = str_split($this->binaryFlags);
        $byte = ord($bytes[$byteIndex]);
        $bytes[$byteIndex] = chr($value ? ($byte | (1 << $bitInByte)) : ($byte & ~(1 << $bitInByte)));
        $this->binaryFlags = implode('', $bytes);
    }
}

// Unpack
class Unpack
{
    public array $titleEntries;
    public array $frames;
    private string $binaryFlags;

    public function __construct(
        string $data,
        public bool $strict = false
    ) {
        $message = Message::fromBytes($data);
        $this->titleEntries = $message->dataBlock->titleEntries;
        $this->binaryFlags = $message->header->binaryFlags;
        $this->frames = $this->parseFrames($message->dataBlock->frames);
    }

    private function parseFrames(array $frames): array
    {
        $parsedFrames = [];
        foreach ($frames as $frame) {
            $frameDict = $this->strict ? array_fill_keys(array_column($this->titleEntries, 'name'), null) : [];
            foreach ($frame->fields as $field) {
                $titleEntry = $this->titleEntries[$field->titleIndex];
                $frameDict[$titleEntry->name] = $this->decodeValue($field->data, $titleEntry->typeCode);
            }
            $parsedFrames[] = $frameDict;
        }
        return $parsedFrames;
    }

    private function decodeValue(string $data, FieldType $typeCode): mixed
    {
        return match ($typeCode) {
            FieldType::STRING => $data,
            FieldType::INT => unpack('q', $data)[1],
            FieldType::FLOAT => unpack('d', $data)[1],
            FieldType::BOOL => ord($data[0]) === 1,
            FieldType::JSON => json_decode($data, true, 512, JSON_THROW_ON_ERROR),
            FieldType::BINARY => $data,
            default => throw new RuntimeException("Unsupported type_code: {$typeCode->value}")
        };
    }

    public function getFlag(int $bitIndex): bool
    {
        if ($bitIndex < 0 || $bitIndex >= 64) {
            throw new ValueError('bit_index must be in 0..63');
        }
        $byteIndex = intdiv($bitIndex, 8);
        $bitInByte = $bitIndex % 8;
        return (bool) ((ord($this->binaryFlags[$byteIndex]) >> $bitInByte) & 1);
    }
}
