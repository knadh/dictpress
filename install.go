package main

import (
	"fmt"
	"strings"
)

func (app *App) installSchema(prompt bool) int {
	if prompt {
		fmt.Println("")
		fmt.Println("** first time installation **")
		fmt.Printf("** IMPORTANT: This will wipe existing dictmaker tables and types in the DB '%s' **",
			ko.String("db.db"))
		fmt.Println("")

		var ok string

		fmt.Print("continue (y/n)?  ")

		if _, err := fmt.Scanf("%s", &ok); err != nil {
			app.logger.Fatalf("error reading value from terminal: %v", err)
		}

		if strings.ToLower(ok) != "y" {
			fmt.Println("install cancelled.")
			return 1
		}
	}

	q, err := app.fs.Read("/schema.sql")
	if err != nil {
		app.logger.Fatal(err.Error())
		return 1
	}

	if _, err := app.db.Exec(string(q)); err != nil {
		app.logger.Fatal(err.Error())
		return 1
	}

	app.logger.Println("successfully installed schema")

	return 0
}
