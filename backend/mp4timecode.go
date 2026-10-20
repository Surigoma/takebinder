package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

type timecodeTable struct {
	deltaOffset int64
	sampleCount uint32
	delta       uint32
}

// extendMergedTimecode keeps the first chapter's continuous SMPTE start value
// and extends its single tmcd sample across the joined recording. mp4-merge
// intentionally keeps only the first tmcd sample, so this repairs only stts;
// media payloads and every other track remain untouched.
func extendMergedTimecode(output string, inputs []videoFile) error {
	if len(inputs) < 2 {
		return errors.New("timecode extension requires multiple inputs")
	}
	var total uint64
	var firstDelta uint32
	for index, input := range inputs {
		file, err := os.Open(input.Path)
		if err != nil {
			return err
		}
		table, findErr := findTimecodeTable(file)
		closeErr := file.Close()
		if findErr != nil {
			return fmt.Errorf("%s timecode: %w", input.Name, findErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if table.sampleCount != 1 || table.delta == 0 {
			return fmt.Errorf("%s has an unsupported timecode table", input.Name)
		}
		if index == 0 {
			firstDelta = table.delta
		}
		total += uint64(table.delta)
	}
	if total > uint64(^uint32(0)) {
		return errors.New("joined timecode duration exceeds MP4 stts capacity")
	}
	file, err := os.OpenFile(output, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	table, err := findTimecodeTable(file)
	if err != nil {
		return fmt.Errorf("output timecode: %w", err)
	}
	if table.sampleCount != 1 || table.delta != firstDelta {
		return errors.New("output timecode does not preserve the first chapter")
	}
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, uint32(total))
	if _, err := file.WriteAt(buffer, table.deltaOffset); err != nil {
		return err
	}
	return file.Sync()
}

func findTimecodeTable(reader io.ReaderAt) (timecodeTable, error) {
	size, err := readerSize(reader)
	if err != nil {
		return timecodeTable{}, err
	}
	var result timecodeTable
	found := false
	err = walkMP4Boxes(reader, 0, size, func(boxType string, payloadStart, boxEnd int64) error {
		if boxType != "trak" {
			return nil
		}
		isTimecode, table, err := inspectTrack(reader, payloadStart, boxEnd)
		if err != nil {
			return err
		}
		if isTimecode {
			if found {
				return errors.New("multiple tmcd tracks are unsupported")
			}
			found, result = true, table
		}
		return nil
	})
	if err != nil {
		return timecodeTable{}, err
	}
	if !found {
		return timecodeTable{}, errors.New("tmcd track is missing")
	}
	return result, nil
}

func inspectTrack(reader io.ReaderAt, start, end int64) (bool, timecodeTable, error) {
	isTimecode := false
	var table timecodeTable
	hasTable := false
	err := walkMP4Boxes(reader, start, end, func(boxType string, payloadStart, boxEnd int64) error {
		switch boxType {
		case "stsd":
			buffer := make([]byte, 16)
			if payloadStart+int64(len(buffer)) > boxEnd {
				return errors.New("truncated stsd box")
			}
			if _, err := reader.ReadAt(buffer, payloadStart); err != nil {
				return err
			}
			entrySize := binary.BigEndian.Uint32(buffer[8:12])
			if entrySize < 8 || payloadStart+8+int64(entrySize) > boxEnd {
				return errors.New("invalid stsd sample entry")
			}
			isTimecode = binary.BigEndian.Uint32(buffer[4:8]) > 0 && string(buffer[12:16]) == "tmcd"
		case "stts":
			buffer := make([]byte, 16)
			if payloadStart+int64(len(buffer)) > boxEnd {
				return errors.New("truncated stts box")
			}
			if _, err := reader.ReadAt(buffer, payloadStart); err != nil {
				return err
			}
			if binary.BigEndian.Uint32(buffer[4:8]) != 1 {
				return errors.New("timecode stts must have one entry")
			}
			table = timecodeTable{deltaOffset: payloadStart + 12, sampleCount: binary.BigEndian.Uint32(buffer[8:12]), delta: binary.BigEndian.Uint32(buffer[12:16])}
			hasTable = true
		}
		return nil
	})
	if err != nil {
		return false, timecodeTable{}, err
	}
	if isTimecode && !hasTable {
		return false, timecodeTable{}, errors.New("tmcd stts is missing")
	}
	return isTimecode, table, nil
}

func walkMP4Boxes(reader io.ReaderAt, start, end int64, visit func(string, int64, int64) error) error {
	for position := start; position < end; {
		header := make([]byte, 8)
		if _, err := reader.ReadAt(header, position); err != nil {
			return err
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		boxType := string(header[4:8])
		headerSize := int64(8)
		if size == 1 {
			extended := make([]byte, 8)
			if _, err := reader.ReadAt(extended, position+8); err != nil {
				return err
			}
			extendedSize := binary.BigEndian.Uint64(extended)
			if extendedSize > uint64(1<<63-1) {
				return fmt.Errorf("invalid %q extended MP4 box size", boxType)
			}
			size = int64(extendedSize)
			headerSize = 16
		} else if size == 0 {
			size = end - position
		}
		if size < headerSize || size > end-position {
			return fmt.Errorf("invalid %q MP4 box size", boxType)
		}
		payloadStart, boxEnd := position+headerSize, position+size
		if err := visit(boxType, payloadStart, boxEnd); err != nil {
			return err
		}
		switch boxType {
		case "moov", "trak", "mdia", "minf", "stbl":
			if err := walkMP4Boxes(reader, payloadStart, boxEnd, visit); err != nil {
				return err
			}
		}
		position = boxEnd
	}
	return nil
}

func readerSize(reader io.ReaderAt) (int64, error) {
	type statter interface{ Stat() (os.FileInfo, error) }
	value, ok := reader.(statter)
	if !ok {
		return 0, errors.New("MP4 reader size is unavailable")
	}
	info, err := value.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
