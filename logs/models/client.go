package models

import (
	"database/sql"
	"time"
)

// AddClient 新增客户端
func AddClient(client *TClient) (int64, error) {
	result, err := DB.Exec(`INSERT INTO t_client
		(ip, port, vkey, info, zip, online, status, created_by, created_time, updated_by, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip,
		client.Online, client.Status,
		client.CreatedBy, client.CreatedTime, client.UpdatedBy, client.UpdatedTime)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteClient 根据Id删除客户端
func DeleteClient(id int64) (int64, error) {
	result, err := DB.Exec("DELETE FROM t_client WHERE id = ?", id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateClient 更新客户端，先查后改
func UpdateClient(client *TClient) (int64, error) {
	result, err := DB.Exec(`UPDATE t_client SET
		ip = ?, port = ?, vkey = ?, info = ?, zip = ?, status = ?, updated_by = ?, updated_time = ?
		WHERE id = ?`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip,
		client.Status, client.UpdatedBy, client.UpdatedTime, client.Id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ChangeClientOnline 更新客户端在线状态
func ChangeClientOnline(client *TClient) (int64, error) {
	result, err := DB.Exec("UPDATE t_client SET online = ?, updated_time = ? WHERE id = ?",
		client.Online, time.Now(), client.Id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ChangeClientStatus 切换客户端状态（启用/禁用）
func ChangeClientStatus(id int64) (int64, error) {
	result, err := DB.Exec("UPDATE t_client SET status = IF(status='1','0','1'), updated_time = ? WHERE id = ?",
		time.Now(), id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ReadClient 根据Id查询客户端
func ReadClient(id int64) (TClient, error) {
	var c TClient
	err := DB.QueryRow(`SELECT id, ip, port, vkey, info, zip, online, status,
		created_by, created_time, updated_by, updated_time FROM t_client WHERE id = ?`, id).
		Scan(&c.Id, &c.Ip, &c.Port, &c.Vkey, &c.Info, &c.Zip, &c.Online, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime)
	return c, err
}

// CheckClient 根据ip、port、vkey校验客户端
func CheckClient(ip, port, vkey string) (TClient, error) {
	var c TClient
	err := DB.QueryRow(`SELECT id, ip, port, vkey, info, zip, online, status,
		created_by, created_time, updated_by, updated_time FROM t_client
		WHERE ip = ? AND port = ? AND vkey = ?`, ip, port, vkey).
		Scan(&c.Id, &c.Ip, &c.Port, &c.Vkey, &c.Info, &c.Zip, &c.Online, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime)
	return c, err
}

// QueryAllClient 查询所有客户端
func QueryAllClient() ([]TClient, error) {
	rows, err := DB.Query(`SELECT id, ip, port, vkey, info, zip, online, status,
		created_by, created_time, updated_by, updated_time FROM t_client`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []TClient
	for rows.Next() {
		var c TClient
		if err := rows.Scan(&c.Id, &c.Ip, &c.Port, &c.Vkey, &c.Info, &c.Zip, &c.Online, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime); err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// QueryPageClient 分页查询所有客户端
func QueryPageClient(pageNum int, pageSize int) (Page, error) {
	offset := (pageNum - 1) * pageSize
	rows, err := DB.Query(`SELECT id, ip, port, vkey, info, zip, online, status,
		created_by, created_time, updated_by, updated_time FROM t_client LIMIT ? OFFSET ?`,
		pageSize, offset)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()

	var clients []TClient
	for rows.Next() {
		var c TClient
		if err := rows.Scan(&c.Id, &c.Ip, &c.Port, &c.Vkey, &c.Info, &c.Zip, &c.Online, &c.Status,
			&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime); err != nil {
			return Page{}, err
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}

	var total int
	if err := DB.QueryRow("SELECT COUNT(*) FROM t_client").Scan(&total); err != nil {
		return Page{}, err
	}
	return PageUtil(total, pageNum, pageSize, clients), nil
}

// scanClient 扫描一行客户端数据
func scanClient(row *sql.Row) (TClient, error) {
	var c TClient
	err := row.Scan(&c.Id, &c.Ip, &c.Port, &c.Vkey, &c.Info, &c.Zip, &c.Online, &c.Status,
		&c.CreatedBy, &c.CreatedTime, &c.UpdatedBy, &c.UpdatedTime)
	return c, err
}
