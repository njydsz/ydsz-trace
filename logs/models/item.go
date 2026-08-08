package models

import (
	"log"
)

// AddItem 新增项目日志项。
func AddItem(item *TItem) (int64, error) {
	res, err := DB.Exec(`INSERT INTO t_item
		(client_id, item_name, item_desc, log_path, log_prefix, log_suffix, status, created_by, created_time, updated_by, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ClientId, item.ItemName, item.ItemDesc, item.LogPath, item.LogPrefix, item.LogSuffix,
		item.Status, item.CreatedBy, item.CreatedTime, item.UpdatedBy, item.UpdatedTime)
	if err != nil {
		log.Printf("insert item err : %v", err)
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Printf("get last insert id err : %v", err)
		return 0, err
	}
	log.Printf("id : %d", id)
	return id, nil
}

// DeleteItem 按 id 删除项目日志项，返回影响行数。
func DeleteItem(id int64) int64 {
	res, err := DB.Exec(`DELETE FROM t_item WHERE id = ?`, id)
	if err != nil {
		log.Printf("delete item err : %v", err)
		return 0
	}
	num, _ := res.RowsAffected()
	return num
}

// UpdateItem 按 id 更新项目日志项全量字段。
func UpdateItem(item *TItem) (int64, error) {
	res, err := DB.Exec(`UPDATE t_item SET
		client_id = ?, item_name = ?, item_desc = ?, log_path = ?, log_prefix = ?, log_suffix = ?, status = ?, updated_time = ?
		WHERE id = ?`,
		item.ClientId, item.ItemName, item.ItemDesc, item.LogPath, item.LogPrefix, item.LogSuffix,
		item.Status, item.UpdatedTime, item.Id)
	if err != nil {
		log.Printf("update item err : %v", err)
		return 0, err
	}
	num, _ := res.RowsAffected()
	log.Printf("update return num : %d", num)
	return num, nil
}

// ChangeItemStatus 切换项目日志状态（0 ↔ 1）。
func ChangeItemStatus(id int64, now string) (int64, error) {
	item := ReadItem(id)
	status := "1"
	if item.Status == "1" {
		status = "0"
	}
	res, err := DB.Exec(`UPDATE t_item SET status = ?, updated_time = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		log.Printf("update item status err : %v", err)
		return 0, err
	}
	num, _ := res.RowsAffected()
	return num, nil
}

// ReadItem 按 id 查询单个项目日志项。
func ReadItem(id int64) (item TItem) {
	err := DB.Get(&item, `SELECT * FROM t_item WHERE id = ?`, id)
	if err != nil {
		log.Printf("read item err: %v", err)
	}
	return item
}

// QueryItemsByClientId 按 client_id 查询所有项目日志。
func QueryItemsByClientId(id int64) ([]TItem, error) {
	items := []TItem{}
	err := DB.Select(&items, `SELECT * FROM t_item WHERE client_id = ? ORDER BY id DESC`, id)
	if err != nil {
		log.Printf("query items by client id err: %v", err)
		return nil, err
	}
	return items, nil
}

// QueryAllItem 查询全部项目日志列表（按 id 倒序）。
func QueryAllItem() ([]TItem, error) {
	items := []TItem{}
	err := DB.Select(&items, `SELECT * FROM t_item ORDER BY id DESC`)
	if err != nil {
		log.Printf("query all item err: %v", err)
		return nil, err
	}
	return items, nil
}

// QueryPageItem 分页查询项目日志列表（LIMIT/OFFSET）。
func QueryPageItem(pageNum int, pageSize int) (page Page) {
	items := []TItem{}
	err := DB.Select(&items, `SELECT * FROM t_item ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (pageNum-1)*pageSize)
	if err != nil {
		log.Printf("query page item err: %v", err)
		return Page{}
	}
	var totalCount int
	if err := DB.Get(&totalCount, `SELECT COUNT(*) FROM t_item`); err != nil {
		log.Printf("count item err: %v", err)
		return Page{}
	}
	page = PageUtil(totalCount, pageNum, pageSize, items)
	return page
}
