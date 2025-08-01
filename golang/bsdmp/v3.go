package bsdmp

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	"github.com/dsnet/compress/bzip2"
	"github.com/ulikunitz/xz/lzma"
)

// Константы и перечисления
const (
	Magic = "BSDM"
)

type FieldType uint8

const (
	FieldTypeString FieldType = 1
	FieldTypeInt    FieldType = 2
	FieldTypeFloat  FieldType = 3
	FieldTypeBool   FieldType = 4
	FieldTypeJSON   FieldType = 5
	FieldTypeBinary FieldType = 6
)

type FieldSize uint8

const (
	FieldSizeB255 FieldSize = 1
	FieldSizeK64  FieldSize = 2
	FieldSizeG4   FieldSize = 4
)

type CompressionMethod uint8

const (
	CompressionNone  CompressionMethod = 0
	CompressionZlib  CompressionMethod = 1
	CompressionGzip  CompressionMethod = 2
	CompressionBzip2 CompressionMethod = 3
	CompressionLZMA  CompressionMethod = 4
)

// Compressor интерфейс для сжатия/распаковки
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

type compressorRegistry map[CompressionMethod]Compressor

var compressors = compressorRegistry{
	CompressionNone:  &noneCompressor{},
	CompressionZlib:  &zlibCompressor{},
	CompressionGzip:  &gzipCompressor{},
	CompressionBzip2: &bzip2Compressor{},
	CompressionLZMA:  &lzmaCompressor{},
}

type noneCompressor struct{}

func (c *noneCompressor) Compress(data []byte) ([]byte, error)   { return data, nil }
func (c *noneCompressor) Decompress(data []byte) ([]byte, error) { return data, nil }

type zlibCompressor struct{}

func (c *zlibCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

func (c *zlibCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

type gzipCompressor struct{}

func (c *gzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

func (c *gzipCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

type bzip2Compressor struct{}

func (c *bzip2Compressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, nil)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *bzip2Compressor) Decompress(data []byte) ([]byte, error) {
	r, err := bzip2.NewReader(bytes.NewReader(data), nil)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

type lzmaCompressor struct{}

func (c *lzmaCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := lzma.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

func (c *lzmaCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// CRCDriver для вычисления и проверки CRC32
type CRCDriver struct{}

func (c *CRCDriver) Check(data []byte, expected uint32) bool {
	return c.Calculate(data) == expected
}

func (c *CRCDriver) Calculate(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// TitleEntry описывает поле
type TitleEntry struct {
	Name     string
	TypeCode FieldType
	TypeLen  FieldSize
}

func (te *TitleEntry) ToBytes() ([]byte, error) {
	nameBytes := []byte(te.Name)
	nameLen := uint16(len(nameBytes))
	if nameLen == 0 {
		return nil, errors.New("empty field name")
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, nameLen)
	buf.Write(nameBytes)
	binary.Write(buf, binary.LittleEndian, te.TypeCode)
	binary.Write(buf, binary.LittleEndian, te.TypeLen)
	return buf.Bytes(), nil
}

func (te *TitleEntry) FromBytes(data []byte) (int, error) {
	if len(data) < 4 {
		return 0, errors.New("insufficient data for title entry")
	}
	nameLen := binary.LittleEndian.Uint16(data[:2])
	if int(2+nameLen+2) > len(data) {
		return 0, errors.New("insufficient data for title entry")
	}
	te.Name = string(data[2 : 2+nameLen])
	te.TypeCode = FieldType(data[2+nameLen])
	te.TypeLen = FieldSize(data[2+nameLen+1])
	return int(2 + nameLen + 2), nil
}

// Field данные поля
type Field struct {
	TitleIndex uint8
	Data       []byte
	TypeLen    FieldSize
}

func (f *Field) ToBytes() ([]byte, error) {
	length := len(f.Data)
	if f.TypeLen == FieldSizeB255 && length > 255 {
		return nil, fmt.Errorf("field length exceeds 255 for type_len=%d", f.TypeLen)
	} else if f.TypeLen == FieldSizeK64 && length > 65535 {
		return nil, fmt.Errorf("field length exceeds 65535 for type_len=%d", f.TypeLen)
	} else if f.TypeLen == FieldSizeG4 && length > math.MaxUint32 {
		return nil, fmt.Errorf("field length exceeds 4GB for type_len=%d", f.TypeLen)
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, f.TitleIndex)
	switch f.TypeLen {
	case FieldSizeB255:
		binary.Write(buf, binary.LittleEndian, uint8(length))
	case FieldSizeK64:
		binary.Write(buf, binary.LittleEndian, uint16(length))
	case FieldSizeG4:
		binary.Write(buf, binary.LittleEndian, uint32(length))
	default:
		return nil, fmt.Errorf("unsupported type_len %d", f.TypeLen)
	}
	buf.Write(f.Data)
	return buf.Bytes(), nil
}

func (f *Field) FromBytes(data []byte, typeLen FieldSize) (int, error) {
	if len(data) < 1 {
		return 0, errors.New("insufficient data for field")
	}
	f.TitleIndex = data[0]
	offset := 1
	var fieldLen uint32
	switch typeLen {
	case FieldSizeB255:
		if len(data) < 2 {
			return 0, errors.New("insufficient data for field length")
		}
		fieldLen = uint32(data[1])
		offset += 1
	case FieldSizeK64:
		if len(data) < 3 {
			return 0, errors.New("insufficient data for field length")
		}
		fieldLen = uint32(binary.LittleEndian.Uint16(data[1:3]))
		offset += 2
	case FieldSizeG4:
		if len(data) < 5 {
			return 0, errors.New("insufficient data for field length")
		}
		fieldLen = binary.LittleEndian.Uint32(data[1:5])
		offset += 4
	default:
		return 0, fmt.Errorf("unsupported type_len %d", typeLen)
	}
	if len(data) < offset+int(fieldLen) {
		return 0, errors.New("insufficient data for field data")
	}
	f.Data = data[offset : offset+int(fieldLen)]
	f.TypeLen = typeLen
	return offset + int(fieldLen), nil
}

// Frame кадр данных
type Frame struct {
	FSE      []byte
	FrameNum uint32
	Fields   []Field
	FEE      []byte
}

func (f *Frame) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Write(f.FSE)
	binary.Write(buf, binary.LittleEndian, f.FrameNum)
	binary.Write(buf, binary.LittleEndian, uint16(len(f.Fields)))
	for _, field := range f.Fields {
		fieldBytes, err := field.ToBytes()
		if err != nil {
			return nil, err
		}
		buf.Write(fieldBytes)
	}
	buf.Write(f.FEE)
	return buf.Bytes(), nil
}

func (f *Frame) FromBytes(data []byte, fse, fee []byte, titles []TitleEntry) (int, error) {
	if !bytes.HasPrefix(data, fse) {
		return 0, errors.New("frame start marker mismatch")
	}
	offset := len(fse)
	if len(data) < offset+6 {
		return 0, errors.New("insufficient data for frame header")
	}
	f.FSE = fse
	f.FrameNum = binary.LittleEndian.Uint32(data[offset : offset+4])
	fieldCount := binary.LittleEndian.Uint16(data[offset+4 : offset+6])
	offset += 6
	f.Fields = make([]Field, 0, fieldCount)
	for i := 0; i < int(fieldCount); i++ {
		var field Field
		if offset >= len(data) {
			return 0, errors.New("insufficient data for fields")
		}
		titleIndex := data[offset]
		if int(titleIndex) >= len(titles) {
			return 0, fmt.Errorf("invalid title index %d", titleIndex)
		}
		n, err := field.FromBytes(data[offset:], titles[titleIndex].TypeLen)
		if err != nil {
			return 0, err
		}
		f.Fields = append(f.Fields, field)
		offset += n
	}
	if !bytes.HasPrefix(data[offset:], fee) {
		return 0, errors.New("frame end marker mismatch")
	}
	f.FEE = fee
	return offset + len(fee), nil
}

// DataBlock блок данных
type DataBlock struct {
	FrameCount   uint32
	FSE          []byte
	FEE          []byte
	TitleEntries []TitleEntry
	Frames       []Frame
}

func (db *DataBlock) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, db.FrameCount)
	binary.Write(buf, binary.LittleEndian, uint32(len(db.FSE)))
	buf.Write(db.FSE)
	binary.Write(buf, binary.LittleEndian, uint32(len(db.FEE)))
	buf.Write(db.FEE)
	var titleBytes bytes.Buffer
	for _, te := range db.TitleEntries {
		tb, err := te.ToBytes()
		if err != nil {
			return nil, err
		}
		titleBytes.Write(tb)
	}
	binary.Write(buf, binary.LittleEndian, uint32(titleBytes.Len()))
	buf.Write(titleBytes.Bytes())
	for _, frame := range db.Frames {
		fb, err := frame.ToBytes()
		if err != nil {
			return nil, err
		}
		buf.Write(fb)
	}
	return buf.Bytes(), nil
}

func (db *DataBlock) FromBytes(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, errors.New("insufficient data for data block")
	}
	db.FrameCount = binary.LittleEndian.Uint32(data[:4])
	fseLen := binary.LittleEndian.Uint32(data[4:8])
	if len(data) < int(8+fseLen) {
		return 0, errors.New("insufficient data for FSE")
	}
	db.FSE = data[8 : 8+fseLen]
	offset := int(8 + fseLen)
	feeLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	if len(data) < offset+int(4+feeLen) {
		return 0, errors.New("insufficient data for FEE")
	}
	db.FEE = data[offset+4 : offset+4+int(feeLen)]
	offset += 4 + int(feeLen)
	titleLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	if len(data) < offset+int(4+titleLen) {
		return 0, errors.New("insufficient data for title")
	}
	offset += 4
	titleData := data[offset : offset+int(titleLen)]
	offset += int(titleLen)
	db.TitleEntries = []TitleEntry{}
	for len(titleData) > 0 {
		var te TitleEntry
		n, err := te.FromBytes(titleData)
		if err != nil {
			return 0, err
		}
		db.TitleEntries = append(db.TitleEntries, te)
		titleData = titleData[n:]
	}
	db.Frames = make([]Frame, 0, db.FrameCount)
	for i := 0; i < int(db.FrameCount); i++ {
		var frame Frame
		n, err := frame.FromBytes(data[offset:], db.FSE, db.FEE, db.TitleEntries)
		if err != nil {
			return 0, err
		}
		db.Frames = append(db.Frames, frame)
		offset += n
	}
	return offset, nil
}

// Header заголовок сообщения
type Header struct {
	Version       uint8
	Extension     uint8
	Compression   CompressionMethod
	Reserved1     uint8
	BinaryFlags   [8]byte
	DataBlockSize uint32
	Reserved2     [8]byte
}

func (h *Header) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, h.Version)
	binary.Write(buf, binary.LittleEndian, h.Extension)
	binary.Write(buf, binary.LittleEndian, h.Compression)
	binary.Write(buf, binary.LittleEndian, h.Reserved1)
	binary.Write(buf, binary.LittleEndian, h.BinaryFlags)
	binary.Write(buf, binary.LittleEndian, h.DataBlockSize)
	binary.Write(buf, binary.LittleEndian, h.Reserved2)
	return buf.Bytes(), nil
}

func (h *Header) FromBytes(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, errors.New("insufficient data for header")
	}
	h.Version = data[0]
	h.Extension = data[1]
	h.Compression = CompressionMethod(data[2])
	h.Reserved1 = data[3]
	copy(h.BinaryFlags[:], data[4:12])
	h.DataBlockSize = binary.LittleEndian.Uint32(data[12:16])
	copy(h.Reserved2[:], data[16:24])
	return 24, nil
}

func (h *Header) SetFlag(bitIndex int, value bool) error {
	if bitIndex < 0 || bitIndex >= 64 {
		return errors.New("bit_index must be in 0..63")
	}
	byteIndex := bitIndex / 8
	bitInByte := bitIndex % 8
	if value {
		h.BinaryFlags[byteIndex] |= 1 << bitInByte
	} else {
		h.BinaryFlags[byteIndex] &= ^(1 << bitInByte)
	}
	return nil
}

func (h *Header) GetFlag(bitIndex int) (bool, error) {
	if bitIndex < 0 || bitIndex >= 64 {
		return false, errors.New("bit_index must be in 0..63")
	}
	byteIndex := bitIndex / 8
	bitInByte := bitIndex % 8
	return (h.BinaryFlags[byteIndex]>>bitInByte)&1 == 1, nil
}

// Message полное сообщение
type Message struct {
	Header    Header
	DataBlock DataBlock
	CRC       uint32
}

func (m *Message) ToBytes() ([]byte, error) {
	dataBytes, err := m.DataBlock.ToBytes()
	if err != nil {
		return nil, err
	}
	comp, ok := compressors[m.Header.Compression]
	if !ok {
		return nil, fmt.Errorf("unsupported compression method %d", m.Header.Compression)
	}
	compressedData, err := comp.Compress(dataBytes)
	if err != nil {
		return nil, err
	}
	m.Header.DataBlockSize = uint32(len(compressedData))
	headerBytes, err := m.Header.ToBytes()
	if err != nil {
		return nil, err
	}
	m.CRC = crc32.ChecksumIEEE(compressedData)
	buf := new(bytes.Buffer)
	buf.Write([]byte(Magic))
	buf.Write(headerBytes)
	buf.Write(compressedData)
	binary.Write(buf, binary.LittleEndian, m.CRC)
	return buf.Bytes(), nil
}

func (m *Message) FromBytes(data []byte) error {
	if len(data) < 4 || string(data[:4]) != Magic {
		return errors.New("invalid BSDMP magic")
	}
	var offset int
	var err error
	offset, err = m.Header.FromBytes(data[4:])
	if err != nil {
		return err
	}
	offset += 4 // Смещение после magic
	if len(data) < offset+int(m.Header.DataBlockSize)+4 {
		return errors.New("data too short for data block and CRC")
	}
	compressedData := data[offset : offset+int(m.Header.DataBlockSize)]
	offset += int(m.Header.DataBlockSize)
	m.CRC = binary.LittleEndian.Uint32(data[offset : offset+4])
	if !(&CRCDriver{}).Check(compressedData, m.CRC) {
		return errors.New("CRC mismatch")
	}
	comp, ok := compressors[m.Header.Compression]
	if !ok {
		return fmt.Errorf("unsupported compression method %d", m.Header.Compression)
	}
	rawData, err := comp.Decompress(compressedData)
	if err != nil {
		return err
	}
	_, err = m.DataBlock.FromBytes(rawData)
	if err != nil {
		return err
	}
	return nil
}

// Pack для создания сообщения
type Pack struct {
	TitleEntries []TitleEntry
	FramesData   [][]interface{}
	Compression  CompressionMethod
	Extension    uint8
	BinaryFlags  [8]byte
	FSE          []byte
	FEE          []byte
}

func NewPack(
	compression CompressionMethod,
	extension uint8,
	binaryFlags *[8]byte,
	fse, fee []byte,
) *Pack {
	p := &Pack{
		Compression: compression,
		Extension:   extension,
		FSE:         fse,
		FEE:         fee,
	}
	if binaryFlags != nil {
		p.BinaryFlags = *binaryFlags
	}
	if len(p.FSE) == 0 {
		p.FSE = []byte{0xFA, 0xFB, 0xFC}
	}
	if len(p.FEE) == 0 {
		p.FEE = []byte{0xFD, 0xFE, 0xFF}
	}
	return p
}

func (p *Pack) Title(fields []struct {
	Name     string
	TypeCode FieldType
	TypeLen  FieldSize
}) error {
	names := make(map[string]struct{})
	for _, f := range fields {
		if _, exists := names[f.Name]; exists {
			return errors.New("field names must be unique")
		}
		names[f.Name] = struct{}{}
	}
	p.TitleEntries = make([]TitleEntry, len(fields))
	for i, f := range fields {
		p.TitleEntries[i] = TitleEntry{
			Name:     f.Name,
			TypeCode: f.TypeCode,
			TypeLen:  f.TypeLen,
		}
	}
	return nil
}

func (p *Pack) Frame(values interface{}) error {
	var vals []interface{}
	switch v := values.(type) {
	case []interface{}:
		if len(v) != len(p.TitleEntries) {
			return errors.New("values count mismatch with title entries")
		}
		vals = v
	case map[string]interface{}:
		vals = make([]interface{}, len(p.TitleEntries))
		for i, te := range p.TitleEntries {
			vals[i] = v[te.Name]
		}
	default:
		return errors.New("unsupported values type")
	}
	p.FramesData = append(p.FramesData, vals)
	return nil
}

func (p *Pack) encodeValue(value interface{}, typeCode FieldType) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch typeCode {
	case FieldTypeString:
		switch v := value.(type) {
		case string:
			return []byte(v), nil
		case []byte:
			return v, nil
		default:
			return []byte(fmt.Sprint(v)), nil
		}
	case FieldTypeInt:
		var i int64
		switch v := value.(type) {
		case int:
			i = int64(v)
		case int64:
			i = v
		default:
			return nil, fmt.Errorf("invalid type for INT: %T", value)
		}
		buf := new(bytes.Buffer)
		binary.Write(buf, binary.LittleEndian, i)
		return buf.Bytes(), nil
	case FieldTypeFloat:
		f, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("invalid type for FLOAT: %T", value)
		}
		buf := new(bytes.Buffer)
		binary.Write(buf, binary.LittleEndian, f)
		return buf.Bytes(), nil
	case FieldTypeBool:
		b := value.(bool)
		if b {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case FieldTypeJSON:
		return json.Marshal(value)
	case FieldTypeBinary:
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid type for BINARY: %T", value)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported type_code %d", typeCode)
	}
}

func (p *Pack) Pack() ([]byte, error) {
	frames := make([]Frame, len(p.FramesData))
	for i, frameValues := range p.FramesData {
		fields := []Field{}
		for idx, value := range frameValues {
			if value == nil {
				continue
			}
			data, err := p.encodeValue(value, p.TitleEntries[idx].TypeCode)
			if err != nil {
				return nil, err
			}
			fields = append(fields, Field{
				TitleIndex: uint8(idx),
				TypeLen:    p.TitleEntries[idx].TypeLen,
				Data:       data,
			})
		}
		frames[i] = Frame{
			FSE:      p.FSE,
			FrameNum: uint32(i + 1),
			Fields:   fields,
			FEE:      p.FEE,
		}
	}
	dataBlock := DataBlock{
		FrameCount:   uint32(len(frames)),
		FSE:          p.FSE,
		FEE:          p.FEE,
		TitleEntries: p.TitleEntries,
		Frames:       frames,
	}
	header := Header{
		Version:       3,
		Extension:     p.Extension,
		Compression:   p.Compression,
		BinaryFlags:   p.BinaryFlags,
		DataBlockSize: 0, // будет обновлено
	}
	message := Message{
		Header:    header,
		DataBlock: dataBlock,
	}
	return message.ToBytes()
}

func (p *Pack) SetFlag(bitIndex int, value bool) error {
	if bitIndex < 0 || bitIndex >= 64 {
		return errors.New("bit_index must be in 0..63")
	}
	byteIndex := bitIndex / 8
	bitInByte := bitIndex % 8
	if value {
		p.BinaryFlags[byteIndex] |= 1 << bitInByte
	} else {
		p.BinaryFlags[byteIndex] &= ^(1 << bitInByte)
	}
	return nil
}

// Unpack для распаковки сообщения
type Unpack struct {
	Message      Message
	TitleEntries []TitleEntry
	Frames       []map[string]interface{}
	Strict       bool
	BinaryFlags  [8]byte
}

func NewUnpack(data []byte, strict bool) (*Unpack, error) {
	var msg Message
	if err := msg.FromBytes(data); err != nil {
		return nil, err
	}
	u := &Unpack{
		Message:      msg,
		TitleEntries: msg.DataBlock.TitleEntries,
		Strict:       strict,
		BinaryFlags:  msg.Header.BinaryFlags,
	}
	u.Frames = u.parseFrames()
	return u, nil
}

func (u *Unpack) GetFlag(bitIndex int) (bool, error) {
	if bitIndex < 0 || bitIndex >= 64 {
		return false, errors.New("bit_index must be in 0..63")
	}
	byteIndex := bitIndex / 8
	bitInByte := bitIndex % 8
	return (u.BinaryFlags[byteIndex]>>bitInByte)&1 == 1, nil
}

func (u *Unpack) decodeValue(data []byte, typeCode FieldType) (interface{}, error) {
	switch typeCode {
	case FieldTypeString:
		return string(data), nil
	case FieldTypeInt:
		if len(data) != 8 {
			return nil, errors.New("invalid length for INT")
		}
		return int64(binary.LittleEndian.Uint64(data)), nil
	case FieldTypeFloat:
		if len(data) != 8 {
			return nil, errors.New("invalid length for FLOAT")
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(data)), nil
	case FieldTypeBool:
		if len(data) != 1 {
			return nil, errors.New("invalid length for BOOL")
		}
		return data[0] != 0, nil
	case FieldTypeJSON:
		var v interface{}
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	case FieldTypeBinary:
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported type_code %d", typeCode)
	}
}

func (u *Unpack) parseFrames() []map[string]interface{} {
	frames := make([]map[string]interface{}, len(u.Message.DataBlock.Frames))
	for i, frame := range u.Message.DataBlock.Frames {
		var frameDict map[string]interface{}
		if u.Strict {
			frameDict = make(map[string]interface{}, len(u.TitleEntries))
			for _, te := range u.TitleEntries {
				frameDict[te.Name] = nil
			}
		} else {
			frameDict = make(map[string]interface{})
		}
		for _, field := range frame.Fields {
			te := u.TitleEntries[field.TitleIndex]
			val, err := u.decodeValue(field.Data, te.TypeCode)
			if err != nil {
				continue // пропускаем ошибочные поля
			}
			frameDict[te.Name] = val
		}
		frames[i] = frameDict
	}
	return frames
}
