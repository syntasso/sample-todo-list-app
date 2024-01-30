# Sample Golang To Do app

The application code is based on a
[blog](https://blog.logrocket.com/building-simple-app-go-postgresql/) published
by Emmanuel John.

## Running

### Running with in-memory store

```bash
go run main.go
```

### Running with postgres

```bash
export PGUSER=<pg user> # defaults to postgres
export PGPASSWORD=<pg password>
export PGSSLMODE=<ssl mode> # defaults to require
export PGHOST=<pg host>
export DBNAME=<db name> # defaults to mydb
```

