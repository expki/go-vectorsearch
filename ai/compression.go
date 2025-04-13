package ai

import (
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
)

var encoder *zstd.Encoder = func() *zstd.Encoder {
	encoder, err := zstd.NewWriter(
		nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
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
	)
	if err != nil {
		panic(err)
	}
	return decoder
}()

func compress(in []byte) (out []byte) {
	out = encoder.EncodeAll(in, out)
	logger.Sugar().Debugf("compressed: %.2f%%", 100*(float32(len(out))/float32(len(in))))
	return out
}

func decompress(in []byte) (out []byte, err error) {
	out, err = decoder.DecodeAll(in, out)
	if err != nil {
		return nil, err
	}
	logger.Sugar().Debugf("decompressed: %.2f%%", 100*(float32(len(in))/float32(len(out))))
	return out, nil
}
