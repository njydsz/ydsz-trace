package models

import (
	"log"
	"time"

	"github.com/astaxie/beego/orm"
	_ "github.com/lib/pq"
)

// AddItem 新增项目日志
func AddItem(item *TItem) (int64, error) {
	o := orm.NewOrm()
	id, err := o.Insert(item)
	if err != nil {
		log.Printf("insert item err : %v", err)
	}
	log.Printf("id : %d", id)
	return id, err
}

// DeleteItem 根据Id删除项目日志
func DeleteItem(id int64) int64 {
	o := orm.NewOrm()
	item := TItem{}
	item.Id = id
	num, _ := o.Delete(&item)
	return num
}

// UpdateItem 更新项目日志，先查后改
func UpdateItem(item *TItem) (int64, error) {
	o := orm.NewOrm()
	c := TItem{}
	c.Id = item.Id
	err := o.Read(&c)
	if o.Read(&c) == nil {
		c.ClientId = item.ClientId
		c.ItemName = item.ItemName
		c.ItemDesc = item.ItemDesc
		c.LogPath = item.LogPath
		c.LogPrefix = item.LogPrefix
		c.LogSuffix = item.LogSuffix
		c.Status = item.Status
		c.UpdatedTime = time.Now()
		if num, err := o.Update(&c); err == nil {
			log.Printf("update return num : %d", num)
			return num, err
		}
	}
	return 0, err
}

// ChangeItemStatus 切换项目状态（启用/禁用）
func ChangeItemStatus(id int64) (int64, error) {
	o := orm.NewOrm()
	c := TItem{}
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

// ReadItem 根据Id查询项目日志
func ReadItem(id int64) (item TItem) {
	o := orm.NewOrm()
	item.Id = id
	err := o.Read(&item)
	if err == orm.ErrNoRows {
		log.Println("查询不到")
	} else if err == orm.ErrMissPK {
		log.Println("找不到主键")
	}
	return item
}

// QueryItemsByClientId 根据客户端ID查询所有项目
func QueryItemsByClientId(id int64) (*[]TItem, error) {
	o := orm.NewOrm()
	items := new([]TItem)
	_, err := o.QueryTable("t_item").Filter("client_id", id).All(items)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return items, nil
}

// QueryAllItem 查询所有项目日志
func QueryAllItem() (*[]TItem, error) {
	o := orm.NewOrm()
	items := new([]TItem)
	_, err := o.QueryTable("t_item").All(items)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return items, nil
}

// QueryPageItem 分页查询所有项目日志
func QueryPageItem(pageNum int, pageSize int) (page Page) {
	o := orm.NewOrm()
	items := new([]TItem)
	o.QueryTable("t_item").Limit(pageSize, (pageNum-1)*pageSize).All(items)
	TotalCount, _ := o.QueryTable("t_item").Count()
	page = PageUtil(int(TotalCount), pageNum, pageSize, items)
	return page
}
