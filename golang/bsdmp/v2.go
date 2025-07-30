package bsdmp

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// CompressionType constants
const (
	CompressionRAW  = 0
	CompressionGZIP = 1
	CompressionZLIB = 2
)

// FieldType constants
const (
	FieldTypeSTRING = 0x01
	FieldTypeINT    = 0x02
	FieldTypeFLOAT  = 0x03
	FieldTypeBOOL   = 0x04
	FieldTypeJSON   = 0x05
)

// Compression utilities
func compressData(data []byte, ctype int) ([]byte, error) {
	switch ctype {
	case CompressionRAW:
		return data, nil
	case CompressionGZIP:
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case CompressionZLIB:
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, errors.New("unknown compression type")
	}
}

func decompressData(data []byte, ctype int) ([]byte, error) {
	switch ctype {
	case CompressionRAW:
		return data, nil
	case CompressionGZIP:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case CompressionZLIB:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	default:
		return nil, errors.New("unknown compression type")
	}
}

// Header structure
type Header struct {
	Version     uint32
	Compression uint32
	DataSize    uint32
	RawData     []byte
}

func (h *Header) Encode() ([]byte, error) {
	compressedData, err := compressData(h.RawData, int(h.Compression))
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, h.Version)
	binary.Write(buf, binary.LittleEndian, h.Compression)
	binary.Write(buf, binary.LittleEndian, uint32(len(compressedData)))
	buf.Write(compressedData)

	return buf.Bytes(), nil
}

func DecodeHeader(data []byte) (*Header, int, error) {
	if len(data) < 12 {
		return nil, 0, errors.New("header too short")
	}

	h := &Header{}
	buf := bytes.NewReader(data[:12])
	binary.Read(buf, binary.LittleEndian, &h.Version)
	binary.Read(buf, binary.LittleEndian, &h.Compression)
	binary.Read(buf, binary.LittleEndian, &h.DataSize)

	if len(data) < 12+int(h.DataSize) {
		return nil, 0, errors.New("data too short for header")
	}

	compressedData := data[12 : 12+h.DataSize]
	rawData, err := decompressData(compressedData, int(h.Compression))
	if err != nil {
		return nil, 0, err
	}

	h.RawData = rawData
	return h, 12 + int(h.DataSize), nil
}

// DataBlock structure
type DataBlock struct {
	FrameCount uint32
	FSE        []byte
	FEE        []byte
	Title      []FieldDescriptor
	Frames     []*Frame
}

type FieldDescriptor struct {
	Name string
	Type byte
}

func (db *DataBlock) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Frame count
	binary.Write(buf, binary.LittleEndian, uint32(len(db.Frames)))

	// FSE
	binary.Write(buf, binary.LittleEndian, uint32(len(db.FSE)))
	buf.Write(db.FSE)

	// FEE
	binary.Write(buf, binary.LittleEndian, uint32(len(db.FEE)))
	buf.Write(db.FEE)

	// Title
	titleData, err := db.encodeTitle()
	if err != nil {
		return nil, err
	}
	binary.Write(buf, binary.LittleEndian, uint32(len(titleData)))
	buf.Write(titleData)

	// Frames
	for _, frame := range db.Frames {
		frameData, err := frame.Encode(db.FSE, db.FEE)
		if err != nil {
			return nil, err
		}
		buf.Write(frameData)
	}

	return buf.Bytes(), nil
}

func (db *DataBlock) encodeTitle() ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, fd := range db.Title {
		nameBytes := []byte(fd.Name)
		binary.Write(buf, binary.LittleEndian, uint16(len(nameBytes)))
		buf.Write(nameBytes)
		binary.Write(buf, binary.LittleEndian, fd.Type)
	}
	return buf.Bytes(), nil
}

func DecodeDataBlock(data []byte) (*DataBlock, error) {
	db := &DataBlock{}
	offset := 0

	// Frame count
	if len(data) < offset+4 {
		return nil, errors.New("data too short for frame count")
	}
	db.FrameCount = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// FSE
	if len(data) < offset+4 {
		return nil, errors.New("data too short for FSE length")
	}
	fseLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(fseLen) {
		return nil, errors.New("data too short for FSE")
	}
	db.FSE = data[offset : offset+int(fseLen)]
	offset += int(fseLen)

	// FEE
	if len(data) < offset+4 {
		return nil, errors.New("data too short for FEE length")
	}
	feeLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(feeLen) {
		return nil, errors.New("data too short for FEE")
	}
	db.FEE = data[offset : offset+int(feeLen)]
	offset += int(feeLen)

	// Title
	if len(data) < offset+4 {
		return nil, errors.New("data too short for title length")
	}
	titleLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(titleLen) {
		return nil, errors.New("data too short for title")
	}
	titleData := data[offset : offset+int(titleLen)]
	offset += int(titleLen)

	title, err := decodeTitle(titleData)
	if err != nil {
		return nil, err
	}
	db.Title = title

	// Frames
	db.Frames = make([]*Frame, 0, db.FrameCount)
	for i := 0; i < int(db.FrameCount); i++ {
		if len(data) <= offset {
			return nil, errors.New("data too short for frames")
		}

		frame, used, err := DecodeFrame(data[offset:], db.FSE, db.FEE)
		if err != nil {
			return nil, err
		}

		db.Frames = append(db.Frames, frame)
		offset += used
	}

	return db, nil
}

func decodeTitle(data []byte) ([]FieldDescriptor, error) {
	var title []FieldDescriptor
	offset := 0

	for offset < len(data) {
		if offset+2 > len(data) {
			return nil, errors.New("title data too short for name length")
		}
		nameLen := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2

		if offset+int(nameLen) > len(data) {
			return nil, errors.New("title data too short for name")
		}
		name := string(data[offset : offset+int(nameLen)])
		offset += int(nameLen)

		if offset >= len(data) {
			return nil, errors.New("title data too short for type")
		}
		fieldType := data[offset]
		offset++

		title = append(title, FieldDescriptor{
			Name: name,
			Type: fieldType,
		})
	}

	return title, nil
}

// Frame structure
type Frame struct {
	FrameNum uint32
	Fields   [][]byte
}

func (f *Frame) Encode(fse, fee []byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	// FSE
	buf.Write(fse)

	// Frame number
	binary.Write(buf, binary.LittleEndian, f.FrameNum)

	// Calculate full length (fields + FEE)
	fieldData := new(bytes.Buffer)
	for _, field := range f.Fields {
		binary.Write(fieldData, binary.LittleEndian, uint16(len(field)))
		fieldData.Write(field)
	}
	fullLen := fieldData.Len() + len(fee)

	// Full length
	binary.Write(buf, binary.LittleEndian, uint32(fullLen))

	// Field data
	buf.Write(fieldData.Bytes())

	// FEE
	buf.Write(fee)

	return buf.Bytes(), nil
}

func DecodeFrame(data, fse, fee []byte) (*Frame, int, error) {
	if !bytes.HasPrefix(data, fse) {
		return nil, 0, errors.New("frame start not found")
	}

	offset := len(fse)

	if len(data) < offset+8 {
		return nil, 0, errors.New("frame too short to contain header")
	}

	frameNum := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	fullLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(fullLen) {
		return nil, 0, errors.New("frame full_len exceeds available data")
	}

	fieldEnd := offset + int(fullLen) - len(fee)
	var fields [][]byte

	for offset+2 <= fieldEnd {
		fieldLen := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2

		if offset+int(fieldLen) > fieldEnd {
			return nil, 0, errors.New("field length exceeds frame boundary")
		}

		field := data[offset : offset+int(fieldLen)]
		offset += int(fieldLen)
		fields = append(fields, field)
	}

	if !bytes.HasPrefix(data[offset:], fee) {
		return nil, 0, errors.New("frame end marker not found")
	}

	offset += len(fee)

	return &Frame{
		FrameNum: frameNum,
		Fields:   fields,
	}, offset, nil
}

// Field encoding/decoding utilities
func DetectFieldType(value interface{}) byte {
	switch value.(type) {
	case bool:
		return FieldTypeBOOL
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return FieldTypeINT
	case float32, float64:
		return FieldTypeFLOAT
	case map[string]interface{}, []interface{}:
		return FieldTypeJSON
	case string:
		return FieldTypeSTRING
	case []byte:
		return FieldTypeSTRING
	default:
		return FieldTypeSTRING
	}
}

func EncodeField(value interface{}, fieldType byte) ([]byte, error) {
	switch fieldType {
	case FieldTypeBOOL:
		b, ok := value.(bool)
		if !ok {
			return nil, errors.New("value is not bool")
		}
		if b {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case FieldTypeINT:
		var intVal int64
		switch v := value.(type) {
		case int:
			intVal = int64(v)
		case int8:
			intVal = int64(v)
		case int16:
			intVal = int64(v)
		case int32:
			intVal = int64(v)
		case int64:
			intVal = v
		case uint:
			intVal = int64(v)
		case uint8:
			intVal = int64(v)
		case uint16:
			intVal = int64(v)
		case uint32:
			intVal = int64(v)
		case uint64:
			// Potential overflow, but we'll proceed anyway
			intVal = int64(v)
		default:
			return nil, errors.New("value is not an integer type")
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(intVal))
		return buf, nil
	case FieldTypeFLOAT:
		var floatVal float64
		switch v := value.(type) {
		case float32:
			floatVal = float64(v)
		case float64:
			floatVal = v
		default:
			return nil, errors.New("value is not a float type")
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, math.Float64bits(floatVal))
		return buf, nil
	case FieldTypeJSON:
		jsonData, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return jsonData, nil
	case FieldTypeSTRING:
		switch v := value.(type) {
		case string:
			return []byte(v), nil
		case []byte:
			return v, nil
		default:
			return []byte(fmt.Sprint(v)), nil
		}
	default:
		return nil, errors.New("unknown field type")
	}
}

func DecodeField(data []byte, fieldType byte) (interface{}, error) {
	switch fieldType {
	case FieldTypeBOOL:
		if len(data) == 0 {
			return false, nil
		}
		return data[0] != 0, nil
	case FieldTypeINT:
		if len(data) < 8 {
			return 0, errors.New("insufficient data for int64")
		}
		return int64(binary.LittleEndian.Uint64(data)), nil
	case FieldTypeFLOAT:
		if len(data) < 8 {
			return 0.0, errors.New("insufficient data for float64")
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(data)), nil
	case FieldTypeJSON:
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	case FieldTypeSTRING:
		return string(data), nil
	default:
		return data, nil
	}
}

// Client implementation
type Client struct {
	Version     uint32
	Compression uint32
	FSE         []byte
	FEE         []byte
	Title       []FieldDescriptor
	Frames      []*Frame
	Counter     uint32
}

func NewClient(version uint32, compression uint32, fse, fee []byte) *Client {
	return &Client{
		Version:     version,
		Compression: compression,
		FSE:         fse,
		FEE:         fee,
		Counter:     1,
	}
}

func (c *Client) Format(fields []string) {
	c.Title = make([]FieldDescriptor, len(fields))
	for i, name := range fields {
		c.Title[i] = FieldDescriptor{
			Name: name,
			Type: FieldTypeSTRING, // Default to string type
		}
	}
}

func (c *Client) FormatWithTypes(fields []FieldDescriptor) {
	c.Title = make([]FieldDescriptor, len(fields))
	copy(c.Title, fields)
}

func (c *Client) AddFrame(values []interface{}) error {
	if len(values) != len(c.Title) {
		return errors.New("number of values doesn't match title definition")
	}

	fields := make([][]byte, len(values))
	for i, val := range values {
		encoded, err := EncodeField(val, c.Title[i].Type)
		if err != nil {
			return err
		}
		fields[i] = encoded
	}

	c.Frames = append(c.Frames, &Frame{
		FrameNum: c.Counter,
		Fields:   fields,
	})
	c.Counter++
	return nil
}

func (c *Client) Encode() ([]byte, error) {
	dataBlock := &DataBlock{
		FrameCount: uint32(len(c.Frames)),
		FSE:        c.FSE,
		FEE:        c.FEE,
		Title:      c.Title,
		Frames:     c.Frames,
	}

	blockData, err := dataBlock.Encode()
	if err != nil {
		return nil, err
	}

	header := &Header{
		Version:     c.Version,
		Compression: c.Compression,
		DataSize:    0, // Will be set during encoding
		RawData:     blockData,
	}

	return header.Encode()
}

// Server implementation
type Server struct {
	Title  []FieldDescriptor
	Frames []map[string]interface{}
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Decode(data []byte) error {
	header, _, err := DecodeHeader(data)
	if err != nil {
		return err
	}

	dataBlock, err := DecodeDataBlock(header.RawData)
	if err != nil {
		return err
	}

	s.Title = dataBlock.Title
	s.Frames = make([]map[string]interface{}, len(dataBlock.Frames))

	for i, frame := range dataBlock.Frames {
		mapped := make(map[string]interface{})
		for j, field := range frame.Fields {
			if j < len(s.Title) {
				val, err := DecodeField(field, s.Title[j].Type)
				if err != nil {
					return err
				}
				mapped[s.Title[j].Name] = val
			} else {
				mapped[strconv.Itoa(j)] = field
			}
		}
		s.Frames[i] = mapped
	}

	return nil
}
