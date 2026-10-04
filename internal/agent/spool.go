package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/protocol"
	"os"
	"strconv"
	"sync"
)

type spoolUsage struct {
	mu      sync.Mutex
	bytes   int64
	records int
}
type logSpool struct {
	journal *executionJournal
	usage   *spoolUsage
	info    os.FileInfo
	size    int64
}

func newSpool(journal *executionJournal, usage *spoolUsage) *logSpool {
	return &logSpool{journal: journal, usage: usage}
}
func (spool *logSpool) pendingPath() string {
	return "spool/" + spool.journal.state.ClaimKey + "-" + strconv.FormatInt(spool.journal.state.LastLogSeq+1, 10) + ".json"
}
func (spool *logSpool) append(record protocol.LogRecord) (protocol.LogChunk, error) {
	journal := spool.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.state.Ref == nil || journal.state.PendingLog != nil || len(record.Text) > 8<<10 {
		return protocol.LogChunk{}, failure("log_backpressure")
	}
	records := []protocol.LogRecord{record}
	data, err := json.Marshal(records)
	if err != nil || len(data) > 64<<10 {
		return protocol.LogChunk{}, failure("log_backpressure")
	}
	spool.usage.mu.Lock()
	defer spool.usage.mu.Unlock()
	if spool.usage.bytes+int64(len(data)) > 64<<20 || spool.usage.records+1 > 8192 {
		return protocol.LogChunk{}, failure("log_backpressure")
	}
	hash := sha256.Sum256(data)
	chunk := protocol.LogChunk{Ref: *journal.state.Ref, Seq: journal.state.LastLogSeq + 1, Offset: journal.state.LastLogOffset, Digest: hex.EncodeToString(hash[:]), Records: records}
	info, err := journal.lock.atomicFile(spool.pendingPath(), data, nil)
	if err != nil {
		return protocol.LogChunk{}, err
	}
	spool.info, spool.size = info, int64(len(data))
	spool.usage.bytes += spool.size
	spool.usage.records++
	journal.state.PendingLog = &chunk
	if err = journal.saveLocked(); err != nil {
		return protocol.LogChunk{}, err
	}
	return chunk, nil
}
func (spool *logSpool) ack(ack protocol.LogAck) error {
	journal := spool.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	pending := journal.state.PendingLog
	if pending == nil || ack.Seq != pending.Seq || ack.Digest != pending.Digest || ack.NextOffset != pending.Offset+spool.size {
		return failure("invalid_response")
	}
	filename := spool.pendingPath()
	journal.state.LastLogSeq, journal.state.LastLogOffset = ack.Seq, ack.NextOffset
	journal.state.PendingLog = nil
	if err := journal.saveLocked(); err != nil {
		return err
	}
	// 中央ACK已持久化到journal之后，才能释放本地字节和容量。
	if err := journal.lock.removeFile(filename, spool.info); err != nil {
		return err
	}
	spool.usage.mu.Lock()
	spool.usage.bytes -= spool.size
	spool.usage.records--
	spool.usage.mu.Unlock()
	spool.info, spool.size = nil, 0
	return nil
}
