package database

import (
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/klauspost/compress/zstd"
)

var encoder *zstd.Encoder = func() *zstd.Encoder {
	encoder, err := zstd.NewWriter(
		nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
		zstd.WithEncoderConcurrency(1),
		zstd.WithLowerEncoderMem(true),
		zstd.WithSingleSegment(true),
	)
	if err != nil {
		panic(err)
	}
	return encoder
}()

var decoder *zstd.Decoder = func() *zstd.Decoder {
	decoder, err := zstd.NewReader(
		nil,
		zstd.IgnoreChecksum(true),
		zstd.WithDecoderLowmem(true),
		zstd.WithDecoderConcurrency(1),
	)
	if err != nil {
		panic(err)
	}
	return decoder
}()

func compress(in []byte) (out []byte) {
	out = encoder.EncodeAll(in, out)
	return out
}

func decompress(in []byte) (out []byte, err error) {
	out, err = decoder.DecodeAll(in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
