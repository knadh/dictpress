package main

import (
	"fmt"
	"os"
	"strings"
)

func installSchema(app *App, prompt bool) {
	if prompt {
		fmt.Println("")
		fmt.Println("** first time installation **")
		fmt.Printf("** IMPORTANT: This will wipe existing dictmaker tables and types in the DB '%s' **",
			ko.String("db.db"))
		fmt.Println("")

		if prompt {
			var ok string
			fmt.Print("continue (y/n)?  ")
			if _, err := fmt.Scanf("%s", &ok); err != nil {
				fmt.Printf("error reading value from terminal: %v", err)
				os.Exit(1)
			}
			if strings.ToLower(ok) != "y" {
				fmt.Println("install cancelled.")
				return
			}
		}
	}

	q, err := app.fs.Read("/schema.sql")
	if err != nil {
		app.logger.Fatal(err.Error())
		return
	}

	if _, err := app.db.Exec(string(q)); err != nil {
		app.logger.Fatal(err.Error())
		return
	}

	app.logger.Println("successfully installed schema")
}
