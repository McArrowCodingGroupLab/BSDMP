<?php

class BSDMP
{
    // Типы сжатия
    public const COMPRESSION_RAW = 0;
    public const COMPRESSION_GZIP = 1;
    public const COMPRESSION_ZLIB = 2;

    // Типы полей
    public const FIELD_STRING = 0x01;
    public const FIELD_INT = 0x02;
    public const FIELD_FLOAT = 0x03;
    public const FIELD_BOOL = 0x04;
    public const FIELD_JSON = 0x05;

    // Утилиты сжатия
    public static function compressData(string $data, int $compressionType): string
    {
        return match ($compressionType) {
            self::COMPRESSION_GZIP => gzencode($data),
            self::COMPRESSION_ZLIB => gzcompress($data),
            default => $data,
        };
    }

    public static function decompressData(string $data, int $compressionType): string
    {
        return match ($compressionType) {
            self::COMPRESSION_GZIP => gzdecode($data),
            self::COMPRESSION_ZLIB => gzuncompress($data),
            default => $data,
        };
    }

    // Определение типа поля
    public static function detectFieldType($value): int
    {
        return match (true) {
            is_bool($value) => self::FIELD_BOOL,
            is_int($value) => self::FIELD_INT,
            is_float($value) => self::FIELD_FLOAT,
            is_array($value) || is_object($value) => self::FIELD_JSON,
            default => self::FIELD_STRING,
        };
    }

    // Кодирование значения поля
    public static function encodeField($value, int $fieldType): string
    {
        return match ($fieldType) {
            self::FIELD_BOOL => $value ? "\x01" : "\x00",
            self::FIELD_INT => pack('q', $value),
            self::FIELD_FLOAT => pack('d', $value),
            self::FIELD_JSON => json_encode($value),
            default => is_string($value) ? $value : strval($value),
        };
    }

    // Декодирование значения поля
    public static function decodeField(string $data, int $fieldType)
    {
        try {
            return match ($fieldType) {
                self::FIELD_BOOL => $data !== "\x00",
                self::FIELD_INT => unpack('q', $data)[1],
                self::FIELD_FLOAT => unpack('d', $data)[1],
                self::FIELD_JSON => json_decode($data, true),
                default => $data,
            };
        } catch (Exception) {
            return $data;
        }
    }
}

class BSDMPHeader
{
    public int $version;
    public int $compression;
    public int $dataSize;
    public string $rawData;

    public function __construct(int $version = 1, int $compression = 0, string $rawData = '')
    {
        $this->version = $version;
        $this->compression = $compression;
        $this->rawData = $rawData;
        $this->dataSize = strlen($rawData);
    }

    public function encode(): string
    {
        $compressedData = BSDMP::compressData($this->rawData, $this->compression);
        $this->dataSize = strlen($compressedData);

        return pack('VVV', $this->version, $this->compression, $this->dataSize) . $compressedData;
    }

    public static function decode(string $data): array
    {
        if (strlen($data) < 12) {
            throw new Exception("Header too short");
        }

        $unpacked = unpack('Vversion/Vcompression/VdataSize', substr($data, 0, 12));
        $header = new self($unpacked['version'], $unpacked['compression']);
        $header->dataSize = $unpacked['dataSize'];

        if (strlen($data) < 12 + $header->dataSize) {
            throw new Exception("Data too short for header");
        }

        $compressedData = substr($data, 12, $header->dataSize);
        $header->rawData = BSDMP::decompressData($compressedData, $header->compression);

        return [$header, 12 + $header->dataSize];
    }
}

class BSDMPDataBlock
{
    public int $frameCount;
    public string $fse;
    public string $fee;
    public array $title;
    public array $frames;

    public function __construct(
        int $frameCount = 0,
        string $fse = 'FRAME>',
        string $fee = '<FRAME>',
        array $title = [],
        array $frames = []
    ) {
        $this->frameCount = $frameCount;
        $this->fse = $fse;
        $this->fee = $fee;
        $this->title = $title;
        $this->frames = $frames;
    }

    public function encodeTitle(): string
    {
        $data = '';
        foreach ($this->title as [$name, $type]) {
            $nameLen = strlen($name);
            $data .= pack('v', $nameLen) . $name . chr($type);
        }
        return $data;
    }

    public function encode(): string
    {
        $this->frameCount = count($this->frames);
        $result = pack('V', $this->frameCount);
        $result .= pack('V', strlen($this->fse)) . $this->fse;
        $result .= pack('V', strlen($this->fee)) . $this->fee;

        $titleData = $this->encodeTitle();
        $result .= pack('V', strlen($titleData)) . $titleData;

        foreach ($this->frames as $frame) {
            $result .= $frame->encode($this->fse, $this->fee);
        }

        return $result;
    }

    public static function decode(string $data): self
    {
        $offset = 0;

        $frameCount = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;

        $fseLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        $fse = substr($data, $offset, $fseLen);
        $offset += $fseLen;

        $feeLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        $fee = substr($data, $offset, $feeLen);
        $offset += $feeLen;

        $titleLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        $titleRaw = substr($data, $offset, $titleLen);
        $offset += $titleLen;

        $title = [];
        $tpos = 0;
        while ($tpos < strlen($titleRaw)) {
            $nlen = unpack('v', substr($titleRaw, $tpos, 2))[1];
            $tpos += 2;
            $name = substr($titleRaw, $tpos, $nlen);
            $tpos += $nlen;
            $tcode = ord(substr($titleRaw, $tpos, 1));
            $tpos += 1;
            $title[] = [$name, $tcode];
        }

        $frames = [];
        for ($i = 0; $i < $frameCount; $i++) {
            $frame = BSDMPFrame::decode(substr($data, $offset), $fse, $fee);
            $frames[] = $frame[0];
            $offset += $frame[1];
        }

        return new self($frameCount, $fse, $fee, $title, $frames);
    }
}

class BSDMPFrame
{
    public int $frameNum;
    public array $fields;

    public function __construct(int $frameNum = 1, array $fields = [])
    {
        $this->frameNum = $frameNum;
        $this->fields = $fields;
    }

    public function encode(string $fse, string $fee): string
    {
        $fieldData = '';
        foreach ($this->fields as $field) {
            $fieldData .= pack('v', strlen($field)) . $field;
        }

        $fullLen = strlen($fieldData) + strlen($fee);

        return $fse
            . pack('V', $this->frameNum)
            . pack('V', $fullLen)
            . $fieldData
            . $fee;
    }

    public static function decode(string $data, string $fse, string $fee): array
    {
        if (strpos($data, $fse) !== 0) {
            throw new Exception("Frame start not found");
        }

        $offset = strlen($fse);

        if (strlen($data) < $offset + 8) {
            throw new Exception("Frame too short to contain header");
        }

        $frameNum = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;
        $fullLen = unpack('V', substr($data, $offset, 4))[1];
        $offset += 4;

        if (strlen($data) < $offset + $fullLen - strlen($fee)) {
            throw new Exception("Frame full_len exceeds available data");
        }

        $fieldEnd = $offset + $fullLen - strlen($fee);
        $fields = [];

        while ($offset + 2 <= $fieldEnd) {
            $flen = unpack('v', substr($data, $offset, 2))[1];
            $offset += 2;

            if ($offset + $flen > $fieldEnd) {
                throw new Exception("Field length exceeds frame boundary");
            }

            $field = substr($data, $offset, $flen);
            $offset += $flen;
            $fields[] = $field;
        }

        if (substr($data, $offset, strlen($fee)) !== $fee) {
            throw new Exception("Frame end marker not found");
        }

        $offset += strlen($fee);
        return [new self($frameNum, $fields), $offset];
    }
}

class BSDMPClient
{
    private int $version;
    private int $compression;
    private string $fse;
    private string $fee;
    private array $frames = [];
    private array $title = [];
    private int $counter = 1;

    public function __construct(
        int $version = 1,
        int $compression = BSDMP::COMPRESSION_RAW,
        string $fse = 'FRAME>',
        string $fee = '<FRAME>'
    ) {
        $this->version = $version;
        $this->compression = $compression;
        $this->fse = $fse;
        $this->fee = $fee;
    }

    public function format(array $fields): void
    {
        $this->title = array_map(fn($name) => [$name, BSDMP::FIELD_STRING], $fields);
    }

    public function formatWithTypes(array $fieldDescriptors): void
    {
        $this->title = $fieldDescriptors;
    }

    public function addFrame(array $values): void
    {
        if (count($values) !== count($this->title)) {
            throw new Exception("Number of values doesn't match title definition");
        }

        $fields = [];
        foreach ($values as $i => $value) {
            $fields[] = BSDMP::encodeField($value, $this->title[$i][1]);
        }

        $this->frames[] = new BSDMPFrame($this->counter++, $fields);
    }

    public function encode(): string
    {
        $dataBlock = new BSDMPDataBlock(
            count($this->frames),
            $this->fse,
            $this->fee,
            $this->title,
            $this->frames
        );

        $blockData = $dataBlock->encode();
        $header = new BSDMPHeader($this->version, $this->compression, $blockData);

        return $header->encode();
    }
}

class BSDMPServer
{
    public array $title = [];
    public array $frames = [];

    public function decode(string $raw): void
    {
        [$header] = BSDMPHeader::decode($raw);
        $dataBlock = BSDMPDataBlock::decode($header->rawData);

        $this->title = $dataBlock->title;
        $this->frames = [];

        foreach ($dataBlock->frames as $frame) {
            $mapped = [];
            foreach ($frame->fields as $i => $field) {
                $name = $i < count($this->title) ? $this->title[$i][0] : strval($i);
                $type = $i < count($this->title) ? $this->title[$i][1] : BSDMP::FIELD_STRING;
                $mapped[$name] = BSDMP::decodeField($field, $type);
            }
            $this->frames[] = $mapped;
        }
    }
}
