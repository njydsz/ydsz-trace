package models

import (
	"log"
	"time"

	"github.com/astaxie/beego/orm"
)

// AddClient 新增客户端
func AddClient(client *TClient) (int64, error) {
	o := orm.NewOrm()
	id, err := o.Insert(client)
	if err != nil {
		log.Printf("insert client err : %v", err)
	}
	log.Printf("id : %d", id)
	return id, err
}

// DeleteClient 根据Id删除客户端
func DeleteClient(id int64) int64 {
	o := orm.NewOrm()
	client := TClient{}
	client.Id = id
	num, _ := o.Delete(&client)
	return num
}

// UpdateClient 更新客户端，先查后改
func UpdateClient(client *TClient) (int64, error) {
	o := orm.NewOrm()
	c := TClient{}
	c.Id = client.Id
	err := o.Read(&c)
	if o.Read(&c) == nil {
		c.Ip = client.Ip
		c.Port = client.Port
		c.Vkey = client.Vkey
		c.Info = client.Info
		c.Zip = client.Zip
		c.Status = client.Status
		c.UpdatedTime = time.Now()
		if num, err := o.Update(&c); err == nil {
			log.Printf("update return num : %d", num)
			return num, err
		}
	}
	return 0, err
}

// ChangeClientOnline 更新客户端在线状态
func ChangeClientOnline(client *TClient) (int64, error) {
	o := orm.NewOrm()
	c := TClient{}
	c.Id = client.Id
	err := o.Read(&c)
	if o.Read(&c) == nil {
		c.Online = client.Online
		c.UpdatedTime = time.Now()
		if num, err := o.Update(&c, "Online", "UpdatedTime"); err == nil {
			log.Printf("update return num : %d", num)
			return num, err
		}
	}
	return 0, err
}

// ChangeClientStatus 切换客户端状态（启用/禁用）
func ChangeClientStatus(id int64) (int64, error) {
	o := orm.NewOrm()
	c := TClient{}
	c.Id = id
	err := o.Read(&c)
	if o.Read(&c) == nil {
		if "1" == c.Status {
			c.Status = "0"
		} else {
			c.Status = "1"
		}
		c.UpdatedTime = time.Now()
		if num, err := o.Update(&c, "Status", "UpdatedTime"); err == nil {
			log.Printf("update return num : %d", num)
			return num, err
		}
	}
	return 0, err
}

// ReadClient 根据Id查询客户端
func ReadClient(id int64) (client TClient) {
	o := orm.NewOrm()
	client.Id = id
	err := o.Read(&client)
	if err == orm.ErrNoRows {
		log.Println("查询不到")
	} else if err == orm.ErrMissPK {
		log.Println("找不到主键")
	}
	return client
}

// CheckClient 根据ip、port、vkey校验客户端
func CheckClient(ip, port, vkey string) (client TClient) {
	o := orm.NewOrm()
	client.Ip = ip
	client.Port = port
	client.Vkey = vkey
	_, err := o.QueryTable("t_client").Filter("ip", ip).Filter("port", port).Filter("vkey", vkey).All(&client)
	if err == orm.ErrNoRows {
		log.Println("查询不到")
	} else if err == orm.ErrMissPK {
		log.Println("找不到主键")
	}
	return client
}

// QueryAllClient 查询所有客户端
func QueryAllClient() ([]TClient, error) {
	o := orm.NewOrm()
	var clients []TClient
	_, err := o.QueryTable("t_client").All(&clients)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return clients, nil
}

// QueryPageClient 分页查询所有客户端
func QueryPageClient(pageNum int, pageSize int) (page Page) {
	o := orm.NewOrm()
	clients := new([]TClient)
	o.QueryTable("t_client").Limit(pageSize, (pageNum-1)*pageSize).All(clients)
	TotalCount, _ := o.QueryTable("t_client").Count()
	page = PageUtil(int(TotalCount), pageNum, pageSize, clients)
	return page
}
