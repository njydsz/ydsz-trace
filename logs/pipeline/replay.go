// Pipeline 子文件：背压批次重放（V2.2-09）。
//
// 职责：把 spool 队列中暂存的统计批次写回数据库。
// 这是「补传」的实际执行者——入队只是暂存，重放才是真正的数据落地。
package pipeline

import (
	"log"

	models "ydsz-trace/logs/models"
)

// replayBatch 将一条暂存批次写回数据库。
//
// 幂等性保证：
//   - BatchUpsertTemplates 使用 INSERT OR REPLACE（按 template_hash 唯一）
//   - RecordPatternHit 为追加型记录，重放会重复累加 count
//
// 为避免重复累加，重放时只恢复模板与样本，不重复写 pattern_stat 计数
// （pattern_stat 的重复对趋势分析影响可接受，而模板丢失会直接影响检索）。
func (p *Pipeline) replayBatch(batch *spoolBatch) error {
	if batch == nil || len(batch.TemplateStats) == 0 {
		return nil
	}

	upserts := make([]*models.TLogTemplate, 0, len(batch.TemplateStats))
	for hash, count := range batch.TemplateStats {
		upserts = append(upserts, &models.TLogTemplate{
			TemplateId: hash,
			TotalCount: count,
		})
	}

	if err := models.BatchUpsertTemplates(upserts); err != nil {
		return err
	}

	// 样本重放：仅在有样本时写一条（用于模板展示，不重复累加计数）
	for _, hit := range batch.PatternHits {
		if hit.Sample == "" {
			continue
		}
		if err := models.RecordPatternHit(batch.ItemID, 0, hit.TemplateHash,
			hit.Timestamp, hit.Sample); err != nil {
			log.Printf("[pipeline] 重放 pattern_hit 失败 hash=%s: %v", hit.TemplateHash, err)
		}
	}

	log.Printf("[pipeline] 重放批次完成: item=%d templates=%d", batch.ItemID, len(upserts))
	return nil
}

// spoolTemplateStats 构造待暂存的统计批次。
func (p *Pipeline) spoolTemplateStats(itemID int64, stats map[string]int,
	samples map[string]string, timestamps map[string]string) *spoolBatch {

	batch := &spoolBatch{
		ItemID:        itemID,
		TemplateStats: stats,
		PatternHits:   make([]spoolPatternHit, 0, len(stats)),
	}

	for hash, sample := range samples {
		if sample == "" {
			continue
		}
		ts := timestamps[hash]
		batch.PatternHits = append(batch.PatternHits, spoolPatternHit{
			TemplateHash: hash,
			Timestamp:    ts,
			Sample:       sample,
		})
	}
	return batch
}
