package teleport

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"reflect"
	"unsafe"
)

type encoder struct {
	output  io.Writer
	scratch [binary.MaxVarintLen64]byte
}

func (encoder *encoder) uint8(v uint8) error {
	encoder.scratch[0] = v
	if _, err := encoder.output.Write(encoder.scratch[:1]); err != nil {
		return err
	}
	return nil
}

func (encoder *encoder) uint16(v uint16) error {
	encoder.scratch[0] = byte(v)
	encoder.scratch[1] = byte(v >> 8)
	if _, err := encoder.output.Write(encoder.scratch[:2]); err != nil {
		return err
	}
	return nil
}

func (encoder *encoder) int64(v int64) error {
	return encoder.uint64(uint64(v))
}

func (encoder *encoder) uint64(v uint64) error {
	encoder.scratch[0] = byte(v)
	encoder.scratch[1] = byte(v >> 8)
	encoder.scratch[2] = byte(v >> 16)
	encoder.scratch[3] = byte(v >> 24)
	encoder.scratch[4] = byte(v >> 32)
	encoder.scratch[5] = byte(v >> 40)
	encoder.scratch[6] = byte(v >> 48)
	encoder.scratch[7] = byte(v >> 56)
	if _, err := encoder.output.Write(encoder.scratch[:8]); err != nil {
		return err
	}
	return nil
}

func (encoder *encoder) string(s string) error {
	if err := encoder.uvarint(uint64(len(s))); err != nil {
		return err
	}
	if _, err := encoder.output.Write(str2bytes(s)); err != nil {
		return err
	}
	return nil
}
func (encoder *encoder) uvarint(v uint64) error {
	ln := binary.PutUvarint(encoder.scratch[:binary.MaxVarintLen64], v)
	if _, err := encoder.output.Write(encoder.scratch[0:ln]); err != nil {
		return err
	}
	return nil
}

func (encoder *encoder) args(args Args) error {
	json, err := json.Marshal(args)
	if err != nil {
		return err
	}
	if err = encoder.uvarint(uint64(len(json))); err != nil {
		return err
	}
	if _, err = encoder.output.Write(json); err != nil {
		return err
	}
	return nil
}
func (encoder *encoder) result(v interface{}) error {
	json, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := encoder.uint8(Data); err != nil {
		return err
	}
	if err := encoder.uvarint(uint64(len(json))); err != nil {
		return err
	}
	if _, err := encoder.output.Write(json); err != nil {
		return err
	}
	return nil
}

type decoder struct {
	input   io.Reader
	scratch [binary.MaxVarintLen64]byte
}

func (decoder *decoder) uint8() (uint8, error) {
	v, err := decoder.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint8(v), nil
}

func (decoder *decoder) uint16() (uint16, error) {
	if _, err := decoder.Read(decoder.scratch[:2]); err != nil {
		return 0, err
	}
	return uint16(decoder.scratch[0]) | uint16(decoder.scratch[1])<<8, nil
}

func (decoder *decoder) int64() (int64, error) {
	v, err := decoder.uint64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func (decoder *decoder) uint64() (uint64, error) {
	if _, err := decoder.Read(decoder.scratch[:8]); err != nil {
		return 0, err
	}
	return uint64(decoder.scratch[0]) |
		uint64(decoder.scratch[1])<<8 |
		uint64(decoder.scratch[2])<<16 |
		uint64(decoder.scratch[3])<<24 |
		uint64(decoder.scratch[4])<<32 |
		uint64(decoder.scratch[5])<<40 |
		uint64(decoder.scratch[6])<<48 |
		uint64(decoder.scratch[7])<<56, nil
}

func (decoder *decoder) uvarint() (uint64, error) {
	return binary.ReadUvarint(decoder)
}

func (decoder *decoder) string() (string, error) {
	strlen, err := decoder.uvarint()
	if err != nil {
		return "", err
	}
	str, err := decoder.fixed(int(strlen))
	if err != nil {
		return "", err
	}
	return string(str), nil
}

func (decoder *decoder) fixed(ln int) ([]byte, error) {
	buf := make([]byte, ln)
	if _, err := decoder.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (decoder *decoder) args(a Args) (Args, error) {
	var (
		args    = reflect.New(reflect.TypeOf(a).Elem()).Interface().(Args)
		ln, err = decoder.uvarint()
	)
	if err != nil {
		return nil, err
	}
	data, err := decoder.fixed(int(ln))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &args); err != nil {
		return nil, err
	}
	return args, nil
}

func (decoder *decoder) result(v interface{}) error {
	ln, err := decoder.uvarint()
	if err != nil {
		return err
	}
	data, err := decoder.fixed(int(ln))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (decoder *decoder) ReadByte() (byte, error) {
	if _, err := decoder.Read(decoder.scratch[:1]); err != nil {
		return 0x0, err
	}
	return decoder.scratch[0], nil
}

func (decoder *decoder) Read(p []byte) (int, error) {
	return io.ReadFull(decoder.input, p)
}

func str2bytes(str string) []byte {
	header := (*reflect.SliceHeader)(unsafe.Pointer(&str))
	header.Len = len(str)
	header.Cap = header.Len
	return *(*[]byte)(unsafe.Pointer(header))
}
