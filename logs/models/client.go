package models

import (
	"log"
)

// AddClient 新增客户端记录。
func AddClient(client *TClient) (int64, error) {
	res, err := DB.Exec(`INSERT INTO t_client
		(ip, port, vkey, info, zip, online, status, created_by, created_time, updated_by, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip, client.Online, client.Status,
		client.CreatedBy, client.CreatedTime, client.UpdatedBy, client.UpdatedTime)
	if err != nil {
		log.Printf("insert client err : %v", err)
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

// DeleteClient 按 id 删除客户端，返回影响行数。
func DeleteClient(id int64) int64 {
	res, err := DB.Exec(`DELETE FROM t_client WHERE id = ?`, id)
	if err != nil {
		log.Printf("delete client err : %v", err)
		return 0
	}
	num, _ := res.RowsAffected()
	return num
}

// UpdateClient 按 id 更新客户端全量字段（不含 created_*）。
func UpdateClient(client *TClient) (int64, error) {
	res, err := DB.Exec(`UPDATE t_client SET
		ip = ?, port = ?, vkey = ?, info = ?, zip = ?, status = ?, updated_time = ?
		WHERE id = ?`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip, client.Status,
		client.UpdatedTime, client.Id)
	if err != nil {
		log.Printf("update client err : %v", err)
		return 0, err
	}
	num, _ := res.RowsAffected()
	log.Printf("update return num : %d", num)
	return num, nil
}

// ChangeClientOnline 更新客户端在线状态（online 字段）。
func ChangeClientOnline(client *TClient) (int64, error) {
	res, err := DB.Exec(`UPDATE t_client SET online = ?, updated_time = ? WHERE id = ?`,
		client.Online, client.UpdatedTime, client.Id)
	if err != nil {
		log.Printf("update client online err : %v", err)
		return 0, err
	}
	num, _ := res.RowsAffected()
	log.Printf("update return num : %d", num)
	return num, nil
}

// ChangeClientStatus 切换客户端启用/禁用状态（0 ↔ 1）。
func ChangeClientStatus(id int64, now string) (int64, error) {
	client := ReadClient(id)
	status := "1"
	if client.Status == "1" {
		status = "0"
	}
	res, err := DB.Exec(`UPDATE t_client SET status = ?, updated_time = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		log.Printf("update client status err : %v", err)
		return 0, err
	}
	num, _ := res.RowsAffected()
	return num, nil
}

// ReadClient 按 id 查询单个客户端。
func ReadClient(id int64) (client TClient) {
	err := DB.Get(&client, `SELECT * FROM t_client WHERE id = ?`, id)
	if err != nil {
		log.Printf("read client err: %v", err)
	}
	return client
}

// CheckClient 按 ip + port + vkey 校验客户端身份。
func CheckClient(ip, port, vkey string) (client TClient) {
	err := DB.Get(&client, `SELECT * FROM t_client WHERE ip = ? AND port = ? AND vkey = ?`, ip, port, vkey)
	if err != nil {
		log.Printf("check client err: %v", err)
	}
	return client
}

// QueryAllClient 查询全部客户端列表（按 id 倒序）。
func QueryAllClient() ([]TClient, error) {
	clients := []TClient{}
	err := DB.Select(&clients, `SELECT * FROM t_client ORDER BY id DESC`)
	if err != nil {
		log.Printf("query all client err: %v", err)
		return nil, err
	}
	return clients, nil
}

// QueryPageClient 分页查询客户端列表（LIMIT/OFFSET）。
func QueryPageClient(pageNum int, pageSize int) (page Page) {
	clients := []TClient{}
	err := DB.Select(&clients, `SELECT * FROM t_client ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (pageNum-1)*pageSize)
	if err != nil {
		log.Printf("query page client err: %v", err)
		return Page{}
	}
	var totalCount int
	if err := DB.Get(&totalCount, `SELECT COUNT(*) FROM t_client`); err != nil {
		log.Printf("count client err: %v", err)
		return Page{}
	}
	page = PageUtil(totalCount, pageNum, pageSize, clients)
	return page
}
