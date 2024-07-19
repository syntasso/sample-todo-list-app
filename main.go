package main

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html"
)

//go:embed views/*
var templates embed.FS

type todo struct {
	Item string
}

type Todoer interface {
	NewTodo(*fiber.Ctx) error
	GetTodos(*fiber.Ctx, string) error
	DeleteTodo(*fiber.Ctx) error
	Healthcheck(c *fiber.Ctx) error
	Init() error
}

type PGDB struct {
	DB *sql.DB
}

func (p *PGDB) GetTodos(ctx *fiber.Ctx, version string) error {
	var res string
	var todos []string
	rows, err := p.DB.Query("SELECT * FROM todos")
	if err != nil {
		log.Fatalln(err)
		ctx.JSON("An error occurred")
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&res)
		todos = append(todos, res)
	}

	return ctx.Render("index", fiber.Map{
		"Todos":      todos,
		"Enterprise": os.Getenv("ENTERPRISE"),
		"Version":    version,
	})
}

func (p *PGDB) NewTodo(ctx *fiber.Ctx) error {
	newTodo := todo{}
	if err := ctx.BodyParser(&newTodo); err != nil {
		log.Printf("An error occurred: %v", err)
		return ctx.SendString(err.Error())
	}
	fmt.Printf("Creating a new To Do: %q\n", newTodo)
	if newTodo.Item != "" {
		_, err := p.DB.Exec("INSERT into todos VALUES ($1)", newTodo.Item)
		if err != nil {
			log.Fatalf("An error occurred while executing query: %v", err)
		}
	}

	return ctx.Redirect("/")
}

func (p *PGDB) DeleteTodo(c *fiber.Ctx) error {
	todoToDelete := c.Query("item")
	p.DB.Exec("DELETE from todos WHERE item=$1", todoToDelete)
	fmt.Printf("Deleting To Do: %q\n", todoToDelete)
	return c.SendString("deleted")
}

func (p *PGDB) Healthcheck(c *fiber.Ctx) error {
	err := p.DB.Ping()
	if err != nil {
		c.SendString(err.Error())
	}
	return err
}

func (p *PGDB) Init() error {
	_, err := p.DB.Exec("CREATE TABLE IF NOT EXISTS todos (item text)")
	return err
}

type LocalDB struct {
	Todos []string
}

func (l *LocalDB) GetTodos(ctx *fiber.Ctx, version string) error {
	return ctx.Render("index", fiber.Map{
		"Todos":      l.Todos,
		"Enterprise": os.Getenv("ENTERPRISE"),
		"Version":    version,
	})
}

func (l *LocalDB) NewTodo(ctx *fiber.Ctx) error {
	newTodo := todo{}
	if err := ctx.BodyParser(&newTodo); err != nil {
		log.Printf("An error occurred: %v", err)
		return ctx.SendString(err.Error())
	}
	fmt.Printf("Creating a new To Do: %q\n", newTodo)
	if newTodo.Item != "" {
		l.Todos = append(l.Todos, newTodo.Item)
	}

	return ctx.Redirect("/")
}

func (l *LocalDB) DeleteTodo(c *fiber.Ctx) error {
	todoToDelete := c.Query("item")
	for i, todo := range l.Todos {
		if strings.EqualFold(todo, todoToDelete) {
			l.Todos = slices.Delete(l.Todos, i, i+1)
		}
	}
	fmt.Printf("Deleting To Do: %q\n", todoToDelete)
	return c.SendString("deleted")
}

func (p *LocalDB) Healthcheck(c *fiber.Ctx) error {
	return nil
}

func (p *LocalDB) Init() error {
	return nil
}

func main() {
	pgUser := or(or(os.Getenv("DB_USER"), os.Getenv("PGUSER")), "postgres")
	pgPassword := or(os.Getenv("DB_PASSWORD"), os.Getenv("PGPASSWORD"))
	pgHost := or(os.Getenv("DB_HOST"), os.Getenv("PGHOST"))
	pgSSLMode := or(or(os.Getenv("DB_SSL_MODE"), os.Getenv("PGSSLMODE")), "require")
	dbName := or(or(os.Getenv("DB_NAME"), os.Getenv("DBNAME")), "mydb")

	version := os.Getenv("VERSION")
	fmt.Println("Version: ", version)

	var querier Todoer
	if pgHost != "" {
		// Connect to database if PGHOST is set
		connStr := fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=%s", pgUser, pgPassword, pgHost, dbName, pgSSLMode)
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		querier = &PGDB{
			DB: db,
		}
	} else {
		querier = &LocalDB{}
	}

	t := http.FS(templates)
	engine := html.NewFileSystem(t, ".html")
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	//checked by kubernetes to see if the pod is ready to receive traffic
	app.Get("/healthz", func(c *fiber.Ctx) error {
		fmt.Println("healthcheck")
		err := querier.Healthcheck(c)
		return err
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return querier.GetTodos(c, version)
	})

	app.Post("/", func(c *fiber.Ctx) error {
		return querier.NewTodo(c)
	})

	app.Delete("/delete", func(c *fiber.Ctx) error {
		return querier.DeleteTodo(c)
	})

	port := or(os.Getenv("PORT"), "8080")
	app.Static("/", "./public")
	app.Use(logger.New())

	//we need to keep re-trying until successful, but don't want to block
	//the api form starting, so we kick off a go-routine
	go func() {
		x := 0
		for {
			log.Println("Attempting to connect to DB")
			var err error
			if err = querier.Init(); err == nil {
				break
			}

			if x > 60 {
				log.Printf("Retried %d times, exiting\n", x)
				log.Fatal(err)
			}

			log.Printf("Failed to connect to DB, retry attempt %d/60. Err: %v\n", x, err)
			time.Sleep(time.Second)
			x++
		}
	}()
	log.Println(app.Listen(fmt.Sprintf(":%v", port)))
}

func or(a string, b string) string {
	if a == "" {
		return b
	}
	return a
}
