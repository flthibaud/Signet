package main

// func (app *application) createLinkHandler(w http.ResponseWriter, r *http.Request) {
// 	var input struct {
// 		Url string `json:"url"`
// 	}

// 	err := app.readJSON(w, r, &input)
// 	if err != nil {
// 		app.badRequestResponse(w, r, err)
// 		return
// 	}

// 	// 1. Validation
// 	v := validator.New()
// 	v.Check(input.Url != "", "url", "must be provided")
// 	v.Check(len(input.Url) <= 2048, "url", "must not be more than 2048 characters long") // URL peut être longue

// 	if !v.Valid() {
// 		app.failedValidationResponse(w, r, v.Errors)
// 		return
// 	}

// 	// 3. Appel au modèle (C'est là que le scraping go-readability se fait !)
// 	// La méthode Insert va :
// 	//  a. Vérifier si la Page existe déjà
// 	//  b. Si non, Scraper l'URL -> Créer la Page
// 	//  c. Créer le Link pour cet User
// 	stats, err := app.models.Link.Insert(input.Url)
// 	if err != nil {
// 		app.serverErrorResponse(w, r, err)
// 		return
// 	}

// 	response := envelope{
// 		"message": "Feed processed successfully",
// 		"feed":    input.Url,
// 		"results": stats,
// 	}

// 	err = app.writeJSON(w, http.StatusOK, response, nil)
// 	if err != nil {
// 		app.serverErrorResponse(w, r, err)
// 	}
// }

// func (app *application) showLinkHandler(w http.ResponseWriter, r *http.Request) {
// 	id, err := app.readIDParam(r)
// 	if err != nil {
// 		http.NotFound(w, r)
// 		return
// 	}

// 	// 2. Récupérer le lien via le modèle
// 	// Important : On passe l'ID du lien ET l'ID du user
// 	// Cela empêche un user de lire les liens d'un autre
// 	link, err := app.models.Link.Get(id)

// 	if err != nil {
// 		switch {
// 		case errors.Is(err, data.ErrRecordNotFound):
// 			http.NotFound(w, r)
// 		default:
// 			app.serverErrorResponse(w, r, err)
// 		}
// 		return
// 	}

// 	// 3. Réponse JSON
// 	err = app.writeJSON(w, http.StatusOK, envelope{"link": link}, nil)
// 	if err != nil {
// 		app.serverErrorResponse(w, r, err)
// 	}
// }
