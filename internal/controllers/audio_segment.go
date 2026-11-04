package controllers

import (
	"encoding/binary"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"
)

func validAudioSegment(startMS, endMS, durationMS int64) bool {
	return (startMS == 0 && endMS == 0) || (startMS >= 0 && endMS > startMS && endMS <= durationMS)
}

func (track AudioTrack) playbackDurationMS() int64 {
	if track.EndMS != 0 {
		return track.EndMS - track.StartMS
	}
	return track.DurationMS
}

func prepareAudioTrackPlayback(track storedAudioTrack) (string, error) {
	if !validAudioSegment(track.StartMS, track.EndMS, track.DurationMS) {
		return "", errors.New("invalid saved audio segment")
	}
	if track.EndMS == 0 {
		return track.StoredFilename, nil
	}
	filename, err := createAudioSegment(track.StoredFilename, track.StartMS, track.EndMS)
	if err != nil {
		return "", err
	}
	path := filepath.Join(audioStoragePath(), filename)
	time.AfterFunc(time.Duration(track.playbackDurationMS())*time.Millisecond+time.Minute, func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("audio: segment cleanup failed: %v", err)
		}
	})
	return filename, nil
}

func audioSegmentWAV(data []byte, startMS, endMS int64) ([]byte, error) {
	normalized, duration, _, _, err := normalizeAudioWAV(data)
	if err != nil {
		return nil, err
	}
	if startMS < 0 || endMS <= startMS || endMS > duration.Milliseconds() {
		return nil, errors.New("invalid audio segment")
	}
	var pcm []byte
	for offset := 12; offset+8 <= len(normalized); {
		size := int(binary.LittleEndian.Uint32(normalized[offset+4:]))
		if string(normalized[offset:offset+4]) == "data" {
			pcm = append(pcm, normalized[offset+8:offset+8+size]...)
		}
		offset += 8 + size + size%2
	}
	start, end := startMS*16, min(endMS*16, int64(len(pcm)))
	if start >= end {
		return nil, errors.New("empty audio segment")
	}
	result := recordingWAVHeader(end - start)
	return append(result, pcm[start:end]...), nil
}

func createAudioSegment(filename string, startMS, endMS int64) (string, error) {
	if !audioFilenamePattern.MatchString(filename) {
		return "", errors.New("invalid stored audio filename")
	}
	if err := prepareAudioFileForPlayback(filename); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(audioStoragePath(), filename))
	if err != nil {
		return "", err
	}
	data, err = audioSegmentWAV(data, startMS, endMS)
	if err != nil {
		return "", err
	}
	name, err := newAudioFilename()
	if err != nil {
		return "", err
	}
	path := filepath.Join(audioStoragePath(), name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		if cleanupErr := os.Remove(path); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			return "", errors.Join(err, cleanupErr)
		}
		return "", err
	}
	return name, nil
}
