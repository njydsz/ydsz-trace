package models

import (
	"log"
	"time"
)

// nowStr 返回当前本地时间（统一格式）。
func nowStrr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// AddClient 新增客户端记录（source_type=identity+virtual_parent 可选）。
func AddClient(client *TClient) (int64, error) {
	res, err := DB.Exec(`INSERT INTO t_client
		(ip, port, vkey, info, zip, online, status, source_type, identity, virtual_parent, labels,
			created_by, created_time, updated_by, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip, client.Online, client.Status,
		client.SourceType, client.Identity, client.VirtualParent, client.Labels,
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

// AddVirtualClient 为 Docker/K8s 模式的 logc 新增一条虚拟客户端记录。
// identity 用作唯一标识；同一个 logc 的多个虚拟客户端拥有相同的 virtual_parent (= logc identifier)。
func AddVirtualClient(client *TClient) (int64, error) {
	// 先检查 identity 是否已存在
	existing := FindVirtualClientByIdentity(client.VirtualParent, client.Identity)
	if existing.Id != 0 {
		// 更新现有记录
		existing.Online = "1"
		existing.UpdatedTime = nowStrr()
		modelsChangeClientOnlineSafe(&existing)
		log.Printf("[virtual] 复用虚拟客户端 identity=%s id=%d", client.Identity, existing.Id)
		return existing.Id, nil
	}
	client.SourceType = orDefault(client.SourceType, "docker")
	client.Status = orDefault(client.Status, "1")
	client.Online = "1"
	client.Zip = orDefault(client.Zip, "1")
	client.CreatedBy = orDefault(client.CreatedBy, "admin")
	client.UpdatedBy = orDefault(client.UpdatedBy, "admin")
	client.Labels = orDefault(client.Labels, "{}")
	now := nowStrr()
	client.CreatedTime = now
	client.UpdatedTime = now

	return AddClient(client)
}

// modelsChangeClientOnlineSafe 更新客户端在线状态（避免与 controllers 的 ChangeClientOnline 同名冲突）。
func modelsChangeClientOnlineSafe(client *TClient) {
	_, err := DB.Exec(`UPDATE t_client SET online = ?, updated_time = ? WHERE id = ?`,
		client.Online, client.UpdatedTime, client.Id)
	if err != nil {
		log.Printf("update virtual client online err: %v", err)
	}
}

// FindVirtualClientByIdentity 按 virtual_parent + identity 查找记录（用于去重）。
func FindVirtualClientByIdentity(virtualParent, identity string) TClient {
	var c TClient
	err := DB.Get(&c, `SELECT * FROM t_client WHERE virtual_parent = ? AND identity = ?`,
		virtualParent, identity)
	if err != nil {
		log.Printf("find virtual client err: %v", err)
	}
	return c
}

// DeleteVirtualClient 按 virtual_parent + identity 删除虚拟客户端。
func DeleteVirtualClient(virtualParent, identity string) int64 {
	res, err := DB.Exec(`DELETE FROM t_client WHERE virtual_parent = ? AND identity = ?`,
		virtualParent, identity)
	if err != nil {
		log.Printf("delete virtual client err: %v", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// orDefault 如果 s 非空返回 s，否则返回 def。
func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
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
		ip = ?, port = ?, vkey = ?, info = ?, zip = ?, status = ?, source_type = ?,
		identity = ?, virtual_parent = ?, labels = ?, updated_time = ?
		WHERE id = ?`,
		client.Ip, client.Port, client.Vkey, client.Info, client.Zip, client.Status, client.SourceType,
		client.Identity, client.VirtualParent, client.Labels,
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

// ChangeClientStatusByIDAndStatus 按 id 设置客户端启用/禁用状态为指定值。
func ChangeClientStatusByIDAndStatus(id int64, status int, now string) (int64, error) {
	statusStr := "0"
	if status == 1 {
		statusStr = "1"
	}
	res, err := DB.Exec(`UPDATE t_client SET status = ?, updated_time = ? WHERE id = ?`,
		statusStr, now, id)
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
// 传 ip 为空则跳过 ip 过滤（兼容 Docker/K8s 模式下通过 identity 认证的场景）。
func CheckClient(ip, port, vkey string) (client TClient) {
	if ip != "" {
		err := DB.Get(&client, `SELECT * FROM t_client WHERE ip = ? AND port = ? AND vkey = ?`, ip, port, vkey)
		if err != nil {
			log.Printf("check client err: %v", err)
		}
		return client
	}
	err := DB.Get(&client, `SELECT * FROM t_client WHERE port = ? AND vkey = ?`, port, vkey)
	if err != nil {
		log.Printf("check client (no ip) err: %v", err)
	}
	return client
}

// CheckClientByIdentity 按 virtual_parent + identity + vkey 查找虚拟客户端。
func CheckClientByIdentity(virtualParent, identity, vkey string) (client TClient) {
	err := DB.Get(&client,
		`SELECT * FROM t_client WHERE virtual_parent = ? AND identity = ? AND vkey = ?`,
		virtualParent, identity, vkey)
	if err != nil {
		log.Printf("check client by identity err: %v", err)
	}
	return client
}

// QueryAllClient 查询全部客户端列表（按 id 倒序，包含 virtual client 和传统 client）。
func QueryAllClient() ([]TClient, error) {
	clients := []TClient{}
	err := DB.Select(&clients, `SELECT * FROM t_client ORDER BY id DESC`)
	if err != nil {
		log.Printf("query all client err: %v", err)
		return nil, err
	}
	return clients, nil
}

// QueryAllTraditionalClient 仅返回 source_type=file 的传统客户端（供前端旧列表使用）。
func QueryAllTraditionalClient() ([]TClient, error) {
	clients := []TClient{}
	err := DB.Select(&clients, `SELECT * FROM t_client WHERE source_type = 'file' ORDER BY id DESC`)
	if err != nil {
		log.Printf("query traditional client err: %v", err)
		return nil, err
	}
	return clients, nil
}

// QueryVirtualClientByParent 返回指定 logc 节点 (virtual_parent) 下的全部虚拟客户端。
func QueryVirtualClientByParent(virtualParent string) ([]TClient, error) {
	clients := []TClient{}
	err := DB.Select(&clients,
		`SELECT * FROM t_client WHERE virtual_parent = ? ORDER BY id DESC`, virtualParent)
	if err != nil {
		log.Printf("query virtual client by parent err: %v", err)
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
