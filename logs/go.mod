module ydsz-trace/logs

go 1.26.5

require (
	github.com/gin-contrib/cors v1.7.7
	github.com/gin-gonic/gin v1.12.0
	github.com/go-sql-driver/mysql v1.9.2
	github.com/jmoiron/sqlx v1.4.0
	github.com/robfig/cron/v3 v3.0.1
)

replace ydsz-trace/pkg => ../pkg
