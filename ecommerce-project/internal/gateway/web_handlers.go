package gateway

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"ecommerce-project/internal/proto"
)

// webTemplateSetup inizializza i template HTML con funzioni custom
func (g *Gateway) parseTemplates() *template.Template {
	// Definiamo una mappa di funzioni personalizzate per l'HTML
	funcMap := template.FuncMap{
		"formatPrice": func(cents int64) string {
			return fmt.Sprintf("%.2f", float64(cents)/100.0)
		},
	}

	// Parsiamo tutti i file .html nella cartella templates
	// Usiamo Must per "crashare" all'avvio se c'è un errore sintattico nei template
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))
	return tmpl
}

// handleWebHome serve la pagina HTML principale del catalogo
func (g *Gateway) handleWebHome(w http.ResponseWriter, r *http.Request) {
	// Compiliamo il template (in produzione andrebbe fatto una volta sola all'avvio del server,
	// ma per sviluppo lo facciamo ad ogni richiesta per vedere le modifiche al volo)
	tmpl := g.parseTemplates()

	// Recuperiamo i prodotti da gRPC (esattamente come fa l'API REST)
	resp, err := g.catalog.ListProducts(context.Background(), &proto.ListProductsRequest{})
	if err != nil {
		http.Error(w, "Failed to communicate with Catalog Service", http.StatusInternalServerError)
		return
	}

	// Creiamo un wrapper per passare i dati al template HTML
	products := resp.GetProducts()

	// Il catalogo contiene uno stock "stale" (non aggiornato dagli ordini). 
	// Chiediamo all'Inventory Service lo stock reale in tempo reale.
	for _, p := range products {
		stockResp, err := g.inventory.GetStock(context.Background(), &proto.GetStockRequest{ProductId: p.Id})
		if err == nil {
			p.Stock = stockResp.Quantity
		}
	}

	data := struct {
		Products []*proto.Product
	}{
		Products: products,
	}

	// Eseguiamo il template fondendolo con i dati
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, "Error rendering HTML page", http.StatusInternalServerError)
	}
}

// handleWebLogin serve la pagina di login
func (g *Gateway) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	tmpl := g.parseTemplates()
	err := tmpl.ExecuteTemplate(w, "login.html", nil)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}

// handleWebRegister serve la pagina di registrazione
func (g *Gateway) handleWebRegister(w http.ResponseWriter, r *http.Request) {
	tmpl := g.parseTemplates()
	err := tmpl.ExecuteTemplate(w, "register.html", nil)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}

// handleWebCart serve la pagina del carrello
func (g *Gateway) handleWebCart(w http.ResponseWriter, r *http.Request) {
	tmpl := g.parseTemplates()
	err := tmpl.ExecuteTemplate(w, "cart.html", nil)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}

// handleWebProduct serve la pagina di dettaglio del singolo prodotto
func (g *Gateway) handleWebProduct(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	resp, err := g.catalog.GetProduct(context.Background(), &proto.GetProductRequest{Id: id})
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	// Recuperiamo lo stock reale dall'Inventory Service
	stockResp, err := g.inventory.GetStock(context.Background(), &proto.GetStockRequest{ProductId: id})
	if err == nil {
		resp.Stock = stockResp.Quantity
	}

	tmpl := g.parseTemplates()
	data := struct {
		Product *proto.Product
	}{
		Product: resp,
	}

	err = tmpl.ExecuteTemplate(w, "product.html", data)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}

// handleWebAdmin serve la pagina di amministrazione
func (g *Gateway) handleWebAdmin(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	tmpl := g.parseTemplates()
	err := tmpl.ExecuteTemplate(w, "admin.html", nil)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}

// handleWebOrders serve la pagina degli ordini dell'utente autenticato.
// Redirige al login se l'utente non è autenticato.
func (g *Gateway) handleWebOrders(w http.ResponseWriter, r *http.Request) {
	tmpl := g.parseTemplates()
	err := tmpl.ExecuteTemplate(w, "orders.html", nil)
	if err != nil {
		http.Error(w, "Rendering error", http.StatusInternalServerError)
	}
}
