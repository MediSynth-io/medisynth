package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/go-chi/chi/v5"
)

func (p *Portal) handleLanding(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "landing.html", "MediSynth", map[string]interface{}{})
}

func (p *Portal) HandleHome(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "home.html", "Home", map[string]interface{}{})
}

func (p *Portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{})
}

func (p *Portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	log.Printf("[REGISTER] Handling register GET request from %s", r.RemoteAddr)

	// Get password requirements
	passwordReqs := auth.GetPasswordRequirements()
	log.Printf("[REGISTER] Password requirements: %+v", passwordReqs)

	data := map[string]interface{}{
		"PasswordRequirements": passwordReqs,
	}

	log.Printf("[REGISTER] Rendering register template with data: %+v", data)
	p.renderTemplate(w, r, "register.html", "Register", data)
}

func (p *Portal) handleLoginRedirect(w http.ResponseWriter, r *http.Request) {
	// Check if we're on the main domain and need to redirect to portal
	if !strings.Contains(r.Host, "portal.") {
		log.Printf("[REDIRECT] Redirecting login from %s to portal.medisynth.io", r.Host)
		redirectURL := "https://" + p.config.DomainPortal + "/login"
		http.Redirect(w, r, redirectURL, http.StatusPermanentRedirect)
		return
	}

	// We're on portal domain, handle normally
	if r.Method == "GET" {
		p.handleLogin(w, r)
	} else {
		p.handleLoginPost(w, r)
	}
}

func (p *Portal) handleRegisterRedirect(w http.ResponseWriter, r *http.Request) {
	log.Printf("[REGISTER_REDIRECT] Method: %s, Host: %s, Path: %s", r.Method, r.Host, r.URL.Path)

	// Check if we're on the main domain and need to redirect to portal
	if !strings.Contains(r.Host, "portal.") {
		log.Printf("[REDIRECT] Redirecting register from %s to portal.medisynth.io", r.Host)
		redirectURL := "https://" + p.config.DomainPortal + "/register"
		http.Redirect(w, r, redirectURL, http.StatusPermanentRedirect)
		return
	}

	log.Printf("[REGISTER_REDIRECT] On portal domain, handling %s request", r.Method)

	// We're on portal domain, handle normally
	if r.Method == "GET" {
		log.Printf("[REGISTER_REDIRECT] Calling handleRegister for GET request")
		p.handleRegister(w, r)
	} else {
		log.Printf("[REGISTER_REDIRECT] Calling handleRegisterPost for %s request", r.Method)
		p.handleRegisterPost(w, r)
	}
}

func (p *Portal) handleAboutRedirect(w http.ResponseWriter, r *http.Request) {
	// Check if we're on the portal domain - redirect to main site about section
	if strings.Contains(r.Host, "portal.") {
		log.Printf("[REDIRECT] Redirecting about from %s to medisynth.io", r.Host)
		redirectURL := "https://medisynth.io/#about"
		http.Redirect(w, r, redirectURL, http.StatusPermanentRedirect)
		return
	}

	// We're on the main domain, show the about content by redirecting to the landing page with anchor
	http.Redirect(w, r, "/#about", http.StatusSeeOther)
}

func (p *Portal) handleDocumentation(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "documentation.html", "Documentation", map[string]interface{}{})
}

func (p *Portal) handleSwaggerProxy(w http.ResponseWriter, r *http.Request) {
	// This creates an authenticated proxy to the API's Swagger UI
	// Only authenticated portal users can access it

	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create API URL using configured internal URL
	apiURL := p.config.APIInternalURL + r.URL.Path
	if r.URL.RawQuery != "" {
		apiURL += "?" + r.URL.RawQuery
	}

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, apiURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers from original request
	for name, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Forward the user's session cookie for API authentication
	if cookie, err := r.Cookie("session"); err == nil {
		proxyReq.AddCookie(cookie)
	}

	// Execute the proxy request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Failed to proxy request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set response status and copy body
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

func (p *Portal) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	log.Printf("[PORTAL] Login attempt for email: %s", email)

	user, err := auth.ValidateUser(email, password)
	if err != nil {
		log.Printf("[PORTAL] User validation failed for %s: %v", email, err)
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{"Error": "Invalid email or password", "Email": email})
		return
	}

	log.Printf("[PORTAL] User validation successful for %s (UserID: %s)", email, user.ID)

	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("ERROR: Session creation failed for user %s: %v", user.ID, err)
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{"Error": "Failed to create session.", "Email": email})
		return
	}

	log.Printf("[PORTAL] Session created successfully for user %s, token length: %d", user.ID, len(token))

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	log.Printf("[PORTAL] Session cookie set for user %s, redirecting to dashboard", user.ID)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	log.Printf("[PORTAL] Registration attempt for email: %s", email)

	data := map[string]interface{}{
		"Email":                email,
		"PasswordRequirements": auth.GetPasswordRequirements(),
	}

	if !auth.ValidateEmail(email) {
		log.Printf("[PORTAL] Invalid email format: %s", email)
		data["Error"] = "Please enter a valid email address"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if password != confirmPassword {
		log.Printf("[PORTAL] Password mismatch for email: %s", email)
		data["Error"] = "Passwords do not match"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if !auth.ValidatePassword(password) {
		log.Printf("[PORTAL] Password validation failed for email: %s", email)
		data["Error"] = "Password does not meet the requirements"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	log.Printf("[PORTAL] Attempting to register user: %s", email)
	user, err := auth.RegisterUser(email, password)
	if err != nil {
		log.Printf("[PORTAL] User registration failed for %s: %v", email, err)
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			data["Error"] = "This email is already registered."
		} else {
			data["Error"] = "Registration failed. Please try again later."
			log.Printf("ERROR: Failed to register user %s: %v", email, err)
		}
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	log.Printf("[PORTAL] User registered successfully: %s (UserID: %s)", email, user.ID)

	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("ERROR: User %s registered but session creation failed: %v", email, err)
		// User is registered, but we can't log them in.
		// Redirect to login with a message.
		http.Redirect(w, r, "/login?info=registration_success", http.StatusSeeOther)
		return
	}

	log.Printf("[PORTAL] Session created successfully for new user %s, token length: %d", user.ID, len(token))

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	log.Printf("[PORTAL] Registration complete for %s, redirecting to dashboard", email)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		log.Printf("Error: userID not found in context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("[DASHBOARD] Rendering dashboard for user: %s", userID)
	log.Printf("[DASHBOARD] Request from host: %s, RemoteAddr: %s", r.Host, r.RemoteAddr)

	// Get API tokens count
	tokens, err := database.GetUserTokens(userID)
	if err != nil {
		log.Printf("[DASHBOARD] Error getting tokens for user %s: %v", userID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get job statistics
	jobs, err := database.GetJobsByUserID(userID)
	if err != nil {
		log.Printf("[DASHBOARD] Error getting jobs for user %s: %v", userID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Calculate real metrics
	totalPatients := 0
	completedJobs := 0
	for _, job := range jobs {
		if job.Status == models.JobStatusCompleted && job.PatientCount != nil {
			totalPatients += *job.PatientCount
			completedJobs++
		}
	}

	log.Printf("[DASHBOARD] Found %d tokens, %d jobs, %d total patients for user %s", len(tokens), len(jobs), totalPatients, userID)

	data := struct {
		APIRequests      int    `json:"apiRequests"`
		RecordsGenerated int    `json:"recordsGenerated"`
		AccountType      string `json:"accountType"`
		TotalJobs        int    `json:"totalJobs"`
		CompletedJobs    int    `json:"completedJobs"`
		ActiveTokens     int    `json:"activeTokens"`
	}{
		APIRequests:      len(jobs), // Each job represents an API request
		RecordsGenerated: totalPatients,
		AccountType:      "Free",
		TotalJobs:        len(jobs),
		CompletedJobs:    completedJobs,
		ActiveTokens:     len(tokens),
	}

	p.renderTemplate(w, r, "dashboard.html", "Dashboard", data)
}

func (p *Portal) handleTokens(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "tokens.html", "API Tokens", nil)
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	name := r.FormValue("name")

	if name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	token, err := auth.CreateToken(userID, name)
	if err != nil {
		log.Printf("Error creating token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to tokens page with the new token value in the query string
	http.Redirect(w, r, "/tokens?new_token="+token.Token, http.StatusSeeOther)
}

func (p *Portal) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	err := auth.DeleteToken(userID, tokenID)
	if err != nil {
		log.Printf("Error deleting token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (p *Portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		auth.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (p *Portal) handleJobs(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		log.Printf("Error: userID not found in context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("[JOBS] Rendering jobs for user: %s", userID)
	log.Printf("[JOBS] Request from host: %s, RemoteAddr: %s", r.Host, r.RemoteAddr)

	jobs, err := database.GetJobsByUserID(userID)
	if err != nil {
		log.Printf("[JOBS] Error getting jobs for user %s: %v", userID, err)
		http.Error(w, "Could not retrieve job history.", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Jobs": jobs,
	}
	p.renderTemplate(w, r, "jobs.html", "Generation History", data)
}

func (p *Portal) handleNewJob(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "new-job.html", "New Job", nil)
}

func (p *Portal) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	params := models.SyntheaParams{
		Population:   toIntPtr(r.FormValue("population")),
		Gender:       toStringPtr(r.FormValue("gender")),
		AgeMin:       toIntPtr(r.FormValue("ageMin")),
		AgeMax:       toIntPtr(r.FormValue("ageMax")),
		State:        toStringPtr(r.FormValue("state")),
		City:         toStringPtr(r.FormValue("city")),
		OutputFormat: toStringPtr(r.FormValue("outputFormat")),
	}

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		log.Printf("ERROR: Failed to marshal job params: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Use configured internal API URL
	apiURL := p.config.APIInternalURL + "/generate-patients"

	// Create the request to the API service
	apiReq, err := http.NewRequestWithContext(r.Context(), "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("ERROR: Failed to create API request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Forward the user's session cookie for authentication
	if cookie, err := r.Cookie("session"); err == nil {
		apiReq.AddCookie(cookie)
	}

	apiReq.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	apiRes, err := client.Do(apiReq)
	if err != nil {
		log.Printf("ERROR: Failed to call API service: %v", err)
		http.Error(w, "Failed to start job. Could not contact API service.", http.StatusInternalServerError)
		return
	}
	defer apiRes.Body.Close()

	if apiRes.StatusCode >= 400 {
		log.Printf("ERROR: API service returned status %d", apiRes.StatusCode)
		http.Error(w, "Failed to create generation job.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

// Helper functions to convert form values to pointers for the SyntheaParams struct
func toIntPtr(s string) *int {
	if s == "" {
		return nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &i
}

func toStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (p *Portal) renderTemplate(w http.ResponseWriter, r *http.Request, tmplName string, pageTitle string, data interface{}) {
	log.Printf("Rendering template: %s", tmplName)

	ts, ok := p.templates[tmplName]
	if !ok {
		log.Printf("Error: template %s not found", tmplName)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create a map to hold template data
	var templateData map[string]interface{}

	// If data is already a map, use it directly
	if existingMap, ok := data.(map[string]interface{}); ok {
		templateData = existingMap
	} else {
		// Otherwise wrap non-nil data in the Data field
		templateData = map[string]interface{}{}
		if data != nil {
			templateData["Data"] = data
		}
	}

	// Add user context and active page info
	templateData["ActivePage"] = pageTitle

	// Only try to fetch user data if there's a userID in the context
	if userID, ok := r.Context().Value("userID").(string); ok && userID != "" {
		user, err := database.GetUserByID(userID)
		if err != nil {
			log.Printf("Warning: Failed to get user data for ID %s: %v", userID, err)
			// Don't fail the template rendering, just log the warning
		} else {
			templateData["User"] = user
			templateData["IsAdmin"] = p.config.IsAdmin(user.Email)
		}
	}

	log.Printf("Executing template %s with data keys: %v", tmplName, getMapKeys(templateData))
	err := ts.ExecuteTemplate(w, "base.html", templateData)
	if err != nil {
		log.Printf("Error rendering template %s: %v", tmplName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	log.Printf("Successfully rendered template: %s", tmplName)
}

// Helper function to get map keys for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// responseWriter is a wrapper around http.ResponseWriter that includes the request context
type responseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (rw *responseWriter) Context() context.Context {
	return rw.req.Context()
}
