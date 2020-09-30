package main

func (app *App) installSchema() int {
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
