// ESM-модуль для Node.js, Deno, Bun и браузера
import { Buffer } from 'node:buffer';
import zlib from 'node:zlib';
import crc32 from 'crc-32';

// Полифилл для браузера
const isNode = typeof process !== 'undefined' && process.versions?.node;
const BufferClass = isNode ? Buffer : globalThis.Buffer || class Buffer extends Uint8Array { };
const TextEncoderClass = globalThis.TextEncoder || class TextEncoder {
    encode(str) { return new Uint8Array([...str].map(c => c.charCodeAt(0))); }
};
const TextDecoderClass = globalThis.TextDecoder || class TextDecoder {
    decode(arr) { return String.fromCharCode(...arr); }
};

// Константы и перечисления
const MAGIC = 'BSDM';

export const FieldType = {
    STRING: 1,
    INT: 2,
    FLOAT: 3,
    BOOL: 4,
    JSON: 5,
    BINARY: 6,
};

export const FieldSize = {
    B255: 1,
    K64: 2,
    G4: 4,
};

export const CompressionMethod = {
    NONE: 0,
    ZLIB: 1,
    GZIP: 2,
    BZIP2: 3,
    LZMA: 4,
};

// Компрессоры
class CompressorRegistry {
    static #compressors = {
        [CompressionMethod.NONE]: class {
            compress(data) { return data; }
            decompress(data) { return data; }
        },
        [CompressionMethod.ZLIB]: class {
            compress(data) {
                if (isNode) return zlib.deflateSync(data, { level: 9 });
                return require('pako').deflate(data, { level: 9 });
            }
            decompress(data) {
                if (isNode) return zlib.inflateSync(data);
                return require('pako').inflate(data);
            }
        },
        [CompressionMethod.GZIP]: class {
            compress(data) {
                if (isNode) return zlib.gzipSync(data, { level: 9 });
                return require('pako').gzip(data, { level: 9 });
            }
            decompress(data) {
                if (isNode) return zlib.gunzipSync(data);
                return require('pako').ungzip(data);
            }
        },
        [CompressionMethod.BZIP2]: class {
            compress() { throw new Error('BZIP2 not supported; use a library like bz2'); }
            decompress() { throw new Error('BZIP2 not supported; use a library like bz2'); }
        },
        [CompressionMethod.LZMA]: class {
            compress() { throw new Error('LZMA not supported; use a library like lzma-purejs'); }
            decompress() { throw new Error('LZMA not supported; use a library like lzma-purejs'); }
        },
    };

    static get(method) {
        const Compressor = this.#compressors[method];
        if (!Compressor) throw new Error(`Unsupported compression method: ${method}`);
        return new Compressor();
    }
}

// CRC32
class CRCDriver {
    static calculate(data) {
        return crc32.buf(data) >>> 0; // Беззнаковое 32-битное число
    }

    static check(data, expected) {
        return (this.calculate(data) >>> 0) === (expected >>> 0);
    }
}

// TitleEntry
class TitleEntry {
    constructor(name, typeCode, typeLen) {
        if (name.length > 65535) throw new Error('Field name too long');
        if (!Object.values(FieldSize).includes(typeLen)) throw new Error('Invalid typeLen');
        this.name = name;
        this.typeCode = typeCode;
        this.typeLen = typeLen;
    }

    toBytes() {
        const nameBytes = new TextEncoderClass().encode(this.name);
        const buffer = Buffer.alloc(2 + nameBytes.length + 2);
        buffer.writeUInt16LE(nameBytes.length, 0);
        buffer.set(nameBytes, 2);
        buffer.writeUInt8(this.typeCode, 2 + nameBytes.length);
        buffer.writeUInt8(this.typeLen, 3 + nameBytes.length);
        return buffer;
    }

    static fromBytes(data, offsetRef) {
        if (offsetRef.offset + 2 > data.length) throw new Error('Insufficient data for name_len');
        const nameLen = data.readUInt16LE(offsetRef.offset);
        offsetRef.offset += 2;
        if (offsetRef.offset + nameLen + 2 > data.length) throw new Error('Insufficient data for title entry');
        const name = new TextDecoderClass().decode(data.subarray(offsetRef.offset, offsetRef.offset + nameLen));
        offsetRef.offset += nameLen;
        const typeCode = data.readUInt8(offsetRef.offset);
        const typeLen = data.readUInt8(offsetRef.offset + 1);
        offsetRef.offset += 2;
        return new TitleEntry(name, typeCode, typeLen);
    }
}

// Field
class Field {
    constructor(titleIndex, data, typeLen) {
        this.titleIndex = titleIndex;
        this.data = data;
        this.typeLen = typeLen;
    }

    toBytes() {
        const length = this.data.length;
        if (this.typeLen === FieldSize.B255 && length > 255) throw new Error('Field length exceeds 255 bytes');
        if (this.typeLen === FieldSize.K64 && length > 65535) throw new Error('Field length exceeds 65535 bytes');
        if (this.typeLen === FieldSize.G4 && length > 0xFFFFFFFF) throw new Error('Field length exceeds 4GB');

        const buffer = Buffer.alloc(1 + this.typeLen + length);
        buffer.writeUInt8(this.titleIndex, 0);
        if (this.typeLen === FieldSize.B255) buffer.writeUInt8(length, 1);
        else if (this.typeLen === FieldSize.K64) buffer.writeUInt16LE(length, 1);
        else if (this.typeLen === FieldSize.G4) buffer.writeUInt32LE(length, 1);
        buffer.set(this.data, 1 + this.typeLen);
        return buffer;
    }

    static fromBytes(data, offsetRef, typeLen) {
        if (offsetRef.offset + 1 > data.length) throw new Error('Insufficient data for title_index');
        const titleIndex = data.readUInt8(offsetRef.offset);
        offsetRef.offset += 1;

        let fieldLen;
        if (typeLen === FieldSize.B255) fieldLen = data.readUInt8(offsetRef.offset);
        else if (typeLen === FieldSize.K64) fieldLen = data.readUInt16LE(offsetRef.offset);
        else if (typeLen === FieldSize.G4) fieldLen = data.readUInt32LE(offsetRef.offset);
        offsetRef.offset += typeLen;

        if (offsetRef.offset + fieldLen > data.length) throw new Error('Insufficient data for field data');
        const fieldData = data.subarray(offsetRef.offset, offsetRef.offset + fieldLen);
        offsetRef.offset += fieldLen;

        return new Field(titleIndex, fieldData, typeLen);
    }
}

// Frame
class Frame {
    constructor(fse, frameNum, fields, fee) {
        this.fse = fse;
        this.frameNum = frameNum;
        this.fields = fields;
        this.fee = fee;
    }

    toBytes() {
        const fieldsBytes = BufferClass.concat(this.fields.map(f => f.toBytes()));
        const buffer = Buffer.alloc(this.fse.length + 6 + fieldsBytes.length + this.fee.length);
        buffer.set(this.fse, 0);
        buffer.writeUInt32LE(this.frameNum, this.fse.length);
        buffer.writeUInt16LE(this.fields.length, this.fse.length + 4);
        buffer.set(fieldsBytes, this.fse.length + 6);
        buffer.set(this.fee, this.fse.length + 6 + fieldsBytes.length);
        return buffer;
    }

    static fromBytes(data, offsetRef, fse, fee, titleEntries) {
        if (!data.subarray(offsetRef.offset, offsetRef.offset + fse.length).equals(fse)) {
            throw new Error('Frame start marker mismatch');
        }
        offsetRef.offset += fse.length;

        if (offsetRef.offset + 6 > data.length) throw new Error('Insufficient data for frame header');
        const frameNum = data.readUInt32LE(offsetRef.offset);
        const fieldCount = data.readUInt16LE(offsetRef.offset + 4);
        offsetRef.offset += 6;

        const fields = [];
        for (let i = 0; i < fieldCount; i++) {
            const titleIndex = data.readUInt8(offsetRef.offset);
            if (!titleEntries[titleIndex]) throw new Error('Invalid title index');
            fields.push(Field.fromBytes(data, offsetRef, titleEntries[titleIndex].typeLen));
        }

        if (!data.subarray(offsetRef.offset, offsetRef.offset + fee.length).equals(fee)) {
            throw new Error('Frame end marker mismatch');
        }
        offsetRef.offset += fee.length;

        return new Frame(fse, frameNum, fields, fee);
    }
}

// DataBlock
class DataBlock {
    constructor(frameCount, fse, fee, titleEntries, frames) {
        this.frameCount = frameCount;
        this.fse = fse;
        this.fee = fee;
        this.titleEntries = titleEntries;
        this.frames = frames;
    }

    toBytes() {
        const titleBytes = BufferClass.concat(this.titleEntries.map(te => te.toBytes()));
        const framesBytes = BufferClass.concat(this.frames.map(f => f.toBytes()));
        const buffer = Buffer.alloc(4 + 4 + this.fse.length + 4 + this.fee.length + 4 + titleBytes.length + framesBytes.length);
        let offset = 0;
        buffer.writeUInt32LE(this.frameCount, offset); offset += 4;
        buffer.writeUInt32LE(this.fse.length, offset); offset += 4;
        buffer.set(this.fse, offset); offset += this.fse.length;
        buffer.writeUInt32LE(this.fee.length, offset); offset += 4;
        buffer.set(this.fee, offset); offset += this.fee.length;
        buffer.writeUInt32LE(titleBytes.length, offset); offset += 4;
        buffer.set(titleBytes, offset); offset += titleBytes.length;
        buffer.set(framesBytes, offset);
        return buffer;
    }

    static fromBytes(data, offsetRef) {
        if (offsetRef.offset + 16 > data.length) throw new Error('Insufficient data for data block');
        const frameCount = data.readUInt32LE(offsetRef.offset);
        const fseLen = data.readUInt32LE(offsetRef.offset + 4);
        offsetRef.offset += 8;
        if (offsetRef.offset + fseLen > data.length) throw new Error('Insufficient data for FSE');
        const fse = data.subarray(offsetRef.offset, offsetRef.offset + fseLen);
        offsetRef.offset += fseLen;
        const feeLen = data.readUInt32LE(offsetRef.offset);
        offsetRef.offset += 4;
        if (offsetRef.offset + feeLen > data.length) throw new Error('Insufficient data for FEE');
        const fee = data.subarray(offsetRef.offset, offsetRef.offset + feeLen);
        offsetRef.offset += feeLen;
        const titleLen = data.readUInt32LE(offsetRef.offset);
        offsetRef.offset += 4;
        if (offsetRef.offset + titleLen > data.length) throw new Error('Insufficient data for title');
        const titleData = data.subarray(offsetRef.offset, offsetRef.offset + titleLen);
        offsetRef.offset += titleLen;

        const titleEntries = [];
        let titleOffset = { offset: 0 };
        while (titleOffset.offset < titleData.length) {
            titleEntries.push(TitleEntry.fromBytes(titleData, titleOffset));
        }

        const frames = [];
        for (let i = 0; i < frameCount; i++) {
            frames.push(Frame.fromBytes(data, offsetRef, fse, fee, titleEntries));
        }

        return new DataBlock(frameCount, fse, fee, titleEntries, frames);
    }
}

// Header
class Header {
    constructor(version, extension, compression, dataBlockSize, binaryFlags = BufferClass.alloc(8), reserved1 = 0, reserved2 = BufferClass.alloc(8)) {
        this.version = version;
        this.extension = extension;
        this.compression = compression;
        this.reserved1 = reserved1;
        this.binaryFlags = binaryFlags;
        this.dataBlockSize = dataBlockSize;
        this.reserved2 = reserved2;
    }

    toBytes() {
        const buffer = Buffer.alloc(24);
        buffer.writeUInt8(this.version, 0);
        buffer.writeUInt8(this.extension, 1);
        buffer.writeUInt8(this.compression, 2);
        buffer.writeUInt8(this.reserved1, 3);
        buffer.set(this.binaryFlags, 4);
        buffer.writeUInt32LE(this.dataBlockSize, 12);
        buffer.set(this.reserved2, 16);
        return buffer;
    }

    static fromBytes(data, offsetRef) {
        if (offsetRef.offset + 24 > data.length) throw new Error('Insufficient data for header');
        const version = data.readUInt8(offsetRef.offset);
        const extension = data.readUInt8(offsetRef.offset + 1);
        const compression = data.readUInt8(offsetRef.offset + 2);
        const reserved1 = data.readUInt8(offsetRef.offset + 3);
        const binaryFlags = data.subarray(offsetRef.offset + 4, offsetRef.offset + 12);
        const dataBlockSize = data.readUInt32LE(offsetRef.offset + 12);
        const reserved2 = data.subarray(offsetRef.offset + 16, offsetRef.offset + 24);
        offsetRef.offset += 24;
        return new Header(version, extension, compression, dataBlockSize, binaryFlags, reserved1, reserved2);
    }

    setFlag(bitIndex, value) {
        if (bitIndex < 0 || bitIndex >= 64) throw new Error('bit_index must be in 0..63');
        const byteIndex = Math.floor(bitIndex / 8);
        const bitInByte = bitIndex % 8;
        const byte = this.binaryFlags.readUInt8(byteIndex);
        this.binaryFlags.writeUInt8(value ? (byte | (1 << bitInByte)) : (byte & ~(1 << bitInByte)), byteIndex);
    }

    getFlag(bitIndex) {
        if (bitIndex < 0 || bitIndex >= 64) throw new Error('bit_index must be in 0..63');
        const byteIndex = Math.floor(bitIndex / 8);
        const bitInByte = bitIndex % 8;
        return ((this.binaryFlags.readUInt8(byteIndex) >> bitInByte) & 1) !== 0;
    }
}

// Message
class Message {
    constructor(header, dataBlock, crc) {
        this.header = header;
        this.dataBlock = dataBlock;
        this.crc = crc;
    }

    toBytes() {
        const rawData = this.dataBlock.toBytes();
        const compressedData = CompressorRegistry.get(this.header.compression).compress(rawData);
        this.header.dataBlockSize = compressedData.length;
        this.crc = CRCDriver.calculate(compressedData);
        return BufferClass.concat([
            BufferClass.from(MAGIC),
            this.header.toBytes(),
            compressedData,
            BufferClass.from([this.crc & 0xFF, (this.crc >> 8) & 0xFF, (this.crc >> 16) & 0xFF, (this.crc >> 24) & 0xFF]),
        ]);
    }

    static fromBytes(data) {
        if (!data.subarray(0, 4).equals(BufferClass.from(MAGIC))) throw new Error('Invalid BSDMP magic');
        const offsetRef = { offset: 4 };
        const header = Header.fromBytes(data, offsetRef);
        if (offsetRef.offset + header.dataBlockSize + 4 > data.length) throw new Error('Data too short for data block and CRC');
        const compressedData = data.subarray(offsetRef.offset, offsetRef.offset + header.dataBlockSize);
        offsetRef.offset += header.dataBlockSize;
        const crc = data.readUInt32LE(offsetRef.offset);
        if (!CRCDriver.check(compressedData, crc)) throw new Error('CRC mismatch');
        const rawData = CompressorRegistry.get(header.compression).decompress(compressedData);
        const dataOffset = { offset: 0 };
        const dataBlock = DataBlock.fromBytes(rawData, dataOffset);
        return new Message(header, dataBlock, crc);
    }
}

// Pack
export class Pack {
    #titleEntries = [];
    #framesData = [];
    #binaryFlags;

    constructor(compression = CompressionMethod.NONE, extension = 0, binaryFlags = null, fse = BufferClass.from([0xFA, 0xFB, 0xFC]), fee = BufferClass.from([0xFD, 0xFE, 0xFF])) {
        this.compression = compression;
        this.extension = extension;
        this.#binaryFlags = binaryFlags || BufferClass.alloc(8);
        this.fse = fse;
        this.fee = fee;
        if (this.#binaryFlags.length !== 8) throw new Error('binary_flags must be 8 bytes');
    }

    title(fields) {
        const names = fields.map(f => f[0]);
        if (new Set(names).size !== names.length) throw new Error('Field names must be unique');
        this.#titleEntries = fields.map(([name, typeCode, typeLen]) => new TitleEntry(name, typeCode, typeLen));
    }

    frame(values) {
        let frameData;
        if (Array.isArray(values)) {
            if (values.length !== this.#titleEntries.length) throw new Error('Values count mismatch with title entries');
            frameData = values;
        } else {
            frameData = this.#titleEntries.map(te => values[te.name] || null);
        }
        this.#framesData.push(frameData);
    }

    pack() {
        const frames = this.#framesData.map((frameValues, i) => {
            const fields = frameValues
                .map((value, idx) => {
                    if (value == null) return null;
                    const titleEntry = this.#titleEntries[idx];
                    const dataBytes = this.#encodeValue(value, titleEntry.typeCode);
                    return new Field(idx, dataBytes, titleEntry.typeLen);
                })
                .filter(f => f);
            return new Frame(this.fse, i + 1, fields, this.fee);
        });

        const dataBlock = new DataBlock(
            frames.length,
            this.fse,
            this.fee,
            this.#titleEntries,
            frames
        );

        const header = new Header(
            3,
            this.extension,
            this.compression,
            0,
            this.#binaryFlags
        );

        return new Message(header, dataBlock, 0).toBytes();
    }

    #encodeValue(value, typeCode) {
        switch (typeCode) {
            case FieldType.STRING: return BufferClass.from(typeof value === 'string' ? value : String(value));
            case FieldType.INT: {
                const buffer = BufferClass.alloc(8);
                buffer.writeBigInt64LE(BigInt(value));
                return buffer;
            }
            case FieldType.FLOAT: {
                const buffer = BufferClass.alloc(8);
                buffer.writeDoubleLE(Number(value));
                return buffer;
            }
            case FieldType.BOOL: return BufferClass.from([value ? 1 : 0]);
            case FieldType.JSON: return BufferClass.from(JSON.stringify(value));
            case FieldType.BINARY: return BufferClass.from(value);
            default: throw new Error(`Unsupported type_code: ${typeCode}`);
        }
    }

    setFlag(bitIndex, value) {
        if (bitIndex < 0 || bitIndex >= 64) throw new Error('bit_index must be in 0..63');
        const byteIndex = Math.floor(bitIndex / 8);
        const bitInByte = bitIndex % 8;
        const byte = this.#binaryFlags.readUInt8(byteIndex);
        this.#binaryFlags.writeUInt8(value ? (byte | (1 << bitInByte)) : (byte & ~(1 << bitInByte)), byteIndex);
    }
}

// Unpack
export class Unpack {
    constructor(data, strict = false) {
        this.message = Message.fromBytes(data);
        this.titleEntries = this.message.dataBlock.titleEntries;
        this.strict = strict;
        this.binaryFlags = this.message.header.binaryFlags;
        this.frames = this.#parseFrames();
    }

    #parseFrames() {
        const frames = [];
        for (const frame of this.message.dataBlock.frames) {
            const frameDict = this.strict ? Object.fromEntries(this.titleEntries.map(te => [te.name, null])) : {};
            for (const field of frame.fields) {
                const titleEntry = this.titleEntries[field.titleIndex];
                frameDict[titleEntry.name] = this.#decodeValue(field.data, titleEntry.typeCode);
            }
            frames.push(frameDict);
        }
        return frames;
    }

    #decodeValue(data, typeCode) {
        switch (typeCode) {
            case FieldType.STRING: return new TextDecoderClass().decode(data);
            case FieldType.INT: return Number(data.readBigInt64LE());
            case FieldType.FLOAT: return data.readDoubleLE();
            case FieldType.BOOL: return data.readUInt8() !== 0;
            case FieldType.JSON: return JSON.parse(new TextDecoderClass().decode(data));
            case FieldType.BINARY: return data;
            default: throw new Error(`Unsupported type_code: ${typeCode}`);
        }
    }

    getFlag(bitIndex) {
        if (bitIndex < 0 || bitIndex >= 64) throw new Error('bit_index must be in 0..63');
        const byteIndex = Math.floor(bitIndex / 8);
        const bitInByte = bitIndex % 8;
        return ((this.binaryFlags.readUInt8(byteIndex) >> bitInByte) & 1) !== 0;
    }
}