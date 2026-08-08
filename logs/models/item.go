package models

import (
	"time"
)

// AddItem 新增项目日志
func AddItem(item *TItem) (int64, error) {
	result, err := DB.Exec(`INSERT INTO t_item
		(client_id, item_name, item_desc, log_path, log_prefix, log_suffix, status,
		created_by, created_time, updated_by, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ClientId, item.ItemName, item.ItemDesc, item.LogPath,
		item.LogPrefix, item.LogSuffix, item.Status,
		item.CreatedBy, item.CreatedTime, item.UpdatedBy, item.UpdatedTime)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteItem 根据Id删除项目日志
func DeleteItem(id int64) (int64, error) {
	result, err := DB.Exec("DELETE FROM t_item WHERE id = ?", id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateItem 更新项目日志，先查后改
func UpdateItem(item *TItem) (int64, error) {
	result, err := DB.Exec(`UPDATE t_item SET
		client_id = ?, item_name = ?, item_desc = ?, log_path = ?, log_prefix = ?, log_suffix = ?,
		status = ?, updated_by = ?, updated_time = ? WHERE id = ?`,
		item.ClientId, item.ItemName, item.ItemDesc, item.LogPath,
		item.LogPrefix, item.LogSuffix, item.Status,
		item.UpdatedBy, item.UpdatedTime, item.Id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ChangeItemStatus 切换项目状态（启用/禁用）
func ChangeItemStatus(id int64) (int64, error) {
	result, err := DB.Exec("UPDATE t_item SET status = IF(status='1','0','1'), updated_time = ? WHERE id = ?",
		time.Now(), id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ReadItem 根据Id查询项目日志
func ReadItem(id int64) (TItem, error) {
	var c TItem
	err := DB.QueryRow(`SELECT id, client_id, item_name, item_desc, log_path, log_prefix, log_suffix,
		status, created_by, created_time, updated_by, updated_time FROM t_item WHERE id = ?`, id).
		Scan(&c.Id, &c.ClientId, &c.ItemName, &c.ItemDesc, &c.LogPath,
			&c.LogPrefix, &c.LogSuffix, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime)
	return c, err
}

// QueryItemsByClientId 根据客户端ID查询所有项目
func QueryItemsByClientId(id int64) ([]TItem, error) {
	rows, err := DB.Query(`SELECT id, client_id, item_name, item_desc, log_path, log_prefix, log_suffix,
		status, created_by, created_time, updated_by, updated_time FROM t_item WHERE client_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TItem
	for rows.Next() {
		var c TItem
		if err := rows.Scan(&c.Id, &c.ClientId, &c.ItemName, &c.ItemDesc, &c.LogPath,
			&c.LogPrefix, &c.LogSuffix, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// QueryAllItem 查询所有项目日志
func QueryAllItem() ([]TItem, error) {
	rows, err := DB.Query(`SELECT id, client_id, item_name, item_desc, log_path, log_prefix, log_suffix,
		status, created_by, created_time, updated_by, updated_time FROM t_item`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TItem
	for rows.Next() {
		var c TItem
		if err := rows.Scan(&c.Id, &c.ClientId, &c.ItemName, &c.ItemDesc, &c.LogPath,
			&c.LogPrefix, &c.LogSuffix, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// QueryPageItem 分页查询所有项目日志
func QueryPageItem(pageNum int, pageSize int) (Page, error) {
	offset := (pageNum - 1) * pageSize
	rows, err := DB.Query(`SELECT id, client_id, item_name, item_desc, log_path, log_prefix, log_suffix,
		status, created_by, created_time, updated_by, updated_time FROM t_item LIMIT ? OFFSET ?`,
		pageSize, offset)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()

	var items []TItem
	for rows.Next() {
		var c TItem
		if err := rows.Scan(&c.Id, &c.ClientId, &c.ItemName, &c.ItemDesc, &c.LogPath,
			&c.LogPrefix, &c.LogSuffix, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime); err != nil {
			return Page{}, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}

	var total int
	if err := DB.QueryRow("SELECT COUNT(*) FROM t_item").Scan(&total); err != nil {
		return Page{}, err
	}
	return PageUtil(total, pageNum, pageSize, items), nil
}
