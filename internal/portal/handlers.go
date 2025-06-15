package portal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/bitcoin"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/MediSynth-io/medisynth/internal/s3"
)

// getRealIP extracts the real client IP from request headers
func getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header first (most common in load balancers)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (used by some proxies)
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Check CF-Connecting-IP (Cloudflare)
	cfip := r.Header.Get("CF-Connecting-IP")
	if cfip != "" {
		return strings.TrimSpace(cfip)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
		ip = ip[:colonIndex]
	}
	return ip
}

// getUserInfo gets user email and ID from request context for logging
func getUserInfo(r *http.Request) (string, string) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		return "", ""
	}

	user, err := database.GetUserByID(userID)
	if err != nil {
		return userID, ""
	}

	return userID, user.Email
}

// logRequest logs request details with real IP and user info
func logRequest(r *http.Request, action string, details ...interface{}) {
	realIP := getRealIP(r)
	userID, email := getUserInfo(r)

	if email != "" {
		log.Printf("[%s] %s - IP: %s, User: %s (%s) %v", action, r.Method+" "+r.URL.Path, realIP, email, userID, details)
	} else if userID != "" {
		log.Printf("[%s] %s - IP: %s, User ID: %s %v", action, r.Method+" "+r.URL.Path, realIP, userID, details)
	} else {
		log.Printf("[%s] %s - IP: %s %v", action, r.Method+" "+r.URL.Path, realIP, details)
	}
}

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

	logRequest(r, "LOGIN", "Login attempt for email:", email)

	user, err := auth.ValidateUser(email, password)
	if err != nil {
		logRequest(r, "LOGIN", "User validation failed for", email, ":", err)
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{"Error": "Invalid email or password", "Email": email})
		return
	}

	logRequest(r, "LOGIN", "User validation successful")

	token, err := auth.CreateSession(user.ID)
	if err != nil {
		logRequest(r, "LOGIN", "Session creation failed:", err)
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{"Error": "Failed to create session.", "Email": email})
		return
	}

	logRequest(r, "LOGIN", "Session created successfully, token length:", len(token))

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

	logRequest(r, "LOGIN", "Session cookie set, redirecting to dashboard")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	logRequest(r, "REGISTER", "Registration attempt for email:", email)

	data := map[string]interface{}{
		"Email":                email,
		"PasswordRequirements": auth.GetPasswordRequirements(),
	}

	if !auth.ValidateEmail(email) {
		logRequest(r, "REGISTER", "Invalid email format:", email)
		data["Error"] = "Please enter a valid email address"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if password != confirmPassword {
		logRequest(r, "REGISTER", "Password mismatch for email:", email)
		data["Error"] = "Passwords do not match"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if !auth.ValidatePassword(password) {
		logRequest(r, "REGISTER", "Password validation failed for email:", email)
		data["Error"] = "Password does not meet the requirements"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	logRequest(r, "REGISTER", "Attempting to register user:", email)
	user, err := auth.RegisterUser(email, password)
	if err != nil {
		logRequest(r, "REGISTER", "User registration failed for", email, ":", err)
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			data["Error"] = "This email is already registered."
		} else {
			data["Error"] = "Registration failed. Please try again later."
			logRequest(r, "REGISTER", "ERROR: Failed to register user", email, ":", err)
		}
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	logRequest(r, "REGISTER", "User registered successfully")

	token, err := auth.CreateSession(user.ID)
	if err != nil {
		logRequest(r, "REGISTER", "User registered but session creation failed:", err)
		// User is registered, but we can't log them in.
		// Redirect to login with a message.
		http.Redirect(w, r, "/login?info=registration_success", http.StatusSeeOther)
		return
	}

	logRequest(r, "REGISTER", "Session created successfully for new user, token length:", len(token))

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

	logRequest(r, "REGISTER", "Registration complete, redirecting to dashboard")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		log.Printf("Error: userID not found in context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logRequest(r, "DASHBOARD", "Rendering dashboard")

	// Get API tokens count
	tokens, err := database.GetUserTokens(userID)
	if err != nil {
		logRequest(r, "DASHBOARD", "Error getting tokens:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get job statistics
	jobs, err := database.GetJobsByUserID(userID)
	if err != nil {
		logRequest(r, "DASHBOARD", "Error getting jobs:", err)
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

	logRequest(r, "DASHBOARD", "Stats:", len(tokens), "tokens,", len(jobs), "jobs,", totalPatients, "patients")

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

func (p *Portal) handlePricing(w http.ResponseWriter, r *http.Request) {
	logRequest(r, "PRICING", "Viewing pricing page")
	p.renderTemplate(w, r, "pricing.html", "Pricing & Donations", nil)
}

func (p *Portal) handleTokens(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "tokens.html", "API Tokens", nil)
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	name := r.FormValue("name")

	if name == "" {
		logRequest(r, "TOKENS", "Token name is required")
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	logRequest(r, "TOKENS", "Creating token with name:", name)

	token, err := auth.CreateToken(userID, name)
	if err != nil {
		logRequest(r, "TOKENS", "Error creating token:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logRequest(r, "TOKENS", "Token created successfully")
	// Redirect to tokens page with the new token value in the query string
	http.Redirect(w, r, "/tokens?new_token="+token.Token, http.StatusSeeOther)
}

func (p *Portal) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		logRequest(r, "TOKENS", "Token ID required for deletion")
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	logRequest(r, "TOKENS", "Deleting token:", tokenID)

	err := auth.DeleteToken(userID, tokenID)
	if err != nil {
		logRequest(r, "TOKENS", "Error deleting token", tokenID, ":", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logRequest(r, "TOKENS", "Token deleted successfully:", tokenID)
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (p *Portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	logRequest(r, "LOGOUT", "User logged out")

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

	logRequest(r, "JOBS", "Rendering jobs list")

	jobs, err := database.GetJobsByUserID(userID)
	if err != nil {
		logRequest(r, "JOBS", "Error getting jobs:", err)
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

// ============================================================================
// ADMIN HANDLERS
// ============================================================================

func (p *Portal) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ADMIN] Rendering admin dashboard")

	// Get admin statistics
	totalUsers, _ := database.GetUserCount()
	totalOrders, _ := database.GetOrderCount()
	totalRevenue, _ := database.GetTotalRevenue()
	recentOrders, _ := database.GetRecentOrders(10)

	data := map[string]interface{}{
		"TotalUsers":   totalUsers,
		"TotalOrders":  totalOrders,
		"TotalRevenue": totalRevenue,
		"RecentOrders": recentOrders,
	}

	p.renderTemplate(w, r, "admin-dashboard.html", "Admin Dashboard", data)
}

func (p *Portal) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	logRequest(r, "ADMIN", "Viewing user management page")
	users, err := database.GetAllUsers()
	if err != nil {
		logRequest(r, "ADMIN", "Error getting all users:", err)
		http.Error(w, "Failed to retrieve users.", http.StatusInternalServerError)
		return
	}
	p.renderTemplate(w, r, "admin-users.html", "User Management", map[string]interface{}{
		"Users": users,
	})
}

func (p *Portal) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	logRequest(r, "ADMIN", "Attempting to delete user:", userID)

	// Prevent admin from deleting themselves
	sessionUserID, _ := r.Context().Value("userID").(string)
	if userID == sessionUserID {
		logRequest(r, "ADMIN", "Admin attempted to self-delete")
		http.Error(w, "You cannot delete your own account.", http.StatusBadRequest)
		return
	}

	if err := database.DeleteUserByID(userID); err != nil {
		logRequest(r, "ADMIN", "Failed to delete user", userID, ":", err)
		http.Error(w, "Failed to delete user.", http.StatusInternalServerError)
		return
	}

	logRequest(r, "ADMIN", "Successfully deleted user:", userID)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (p *Portal) handleAdminForcePasswordReset(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	logRequest(r, "ADMIN", "Attempting to force password reset for user:", userID)

	if err := database.SetForcePasswordReset(userID, true); err != nil {
		logRequest(r, "ADMIN", "Failed to force password reset for user", userID, ":", err)
		http.Error(w, "Failed to set password reset flag.", http.StatusInternalServerError)
		return
	}

	logRequest(r, "ADMIN", "Successfully flagged user for password reset:", userID)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (p *Portal) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ADMIN] Rendering admin orders page")

	orders, err := database.GetAllOrders()
	if err != nil {
		log.Printf("[ADMIN] Error getting orders: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Orders": orders,
	}

	p.renderTemplate(w, r, "admin-orders.html", "Admin Orders", data)
}

func (p *Portal) handleAdminPayments(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ADMIN] Rendering admin payments page")

	payments, err := database.GetAllPayments()
	if err != nil {
		log.Printf("[ADMIN] Error getting payments: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Payments": payments,
	}

	p.renderTemplate(w, r, "admin-payments.html", "Admin Payments", data)
}

func (p *Portal) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	log.Printf("[ADMIN] Creating new order")

	// Parse form data
	userID := r.FormValue("user_id")
	description := r.FormValue("description")
	amountUSD := r.FormValue("amount_usd")

	if userID == "" || description == "" || amountUSD == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	amount, err := strconv.ParseFloat(amountUSD, 64)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	// Use Bitcoin service to process the order with payment setup
	bitcoinService := bitcoin.NewBitcoinService()
	order, err := bitcoinService.ProcessOrderPayment(userID, description, amount, p.config.BitcoinAddress)
	if err != nil {
		log.Printf("[ADMIN] Error creating order: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("[ADMIN] Created order %s for user %s", order.OrderNumber, userID)
	http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
}

// ============================================================================
// USER ORDER HANDLERS
// ============================================================================

func (p *Portal) handleUserOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logRequest(r, "ORDERS", "Viewing orders list")

	orders, err := database.GetUserOrders(userID)
	if err != nil {
		logRequest(r, "ORDERS", "Error getting orders:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Orders": orders,
	}

	p.renderTemplate(w, r, "orders.html", "My Orders", data)
}

func (p *Portal) handleOrderDetails(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		http.Error(w, "Order ID required", http.StatusBadRequest)
		return
	}

	logRequest(r, "ORDERS", "Viewing order details for:", orderID)

	order, err := database.GetOrderByID(orderID, userID)
	if err != nil {
		logRequest(r, "ORDERS", "Error getting order", orderID+":", err)
		if err == sql.ErrNoRows {
			http.Error(w, "Order not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	data := map[string]interface{}{
		"Order": order,
	}

	p.renderTemplate(w, r, "order-details.html", "Order Details", data)
}

func (p *Portal) handleCreateUserOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logRequest(r, "ORDERS", "Creating new order")

	// Parse form data
	description := r.FormValue("description")
	amountUSD := r.FormValue("amount_usd")

	if description == "" || amountUSD == "" {
		logRequest(r, "ORDERS", "Missing required fields")
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	amount, err := strconv.ParseFloat(amountUSD, 64)
	if err != nil {
		logRequest(r, "ORDERS", "Invalid amount:", amountUSD)
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	logRequest(r, "ORDERS", "Processing payment for $"+amountUSD)

	// Use Bitcoin service to process the order with payment setup
	bitcoinService := bitcoin.NewBitcoinService()
	order, err := bitcoinService.ProcessOrderPayment(userID, description, amount, p.config.BitcoinAddress)
	if err != nil {
		logRequest(r, "ORDERS", "Error creating order:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logRequest(r, "ORDERS", "Created order", order.OrderNumber, "successfully")
	http.Redirect(w, r, "/orders/"+order.ID, http.StatusSeeOther)
}

func (p *Portal) handleCreateOrderForm(w http.ResponseWriter, r *http.Request) {
	logRequest(r, "ORDERS", "Showing order creation form")
	p.renderTemplate(w, r, "create-order.html", "Create Order", nil)
}

// responseWriter is a wrapper around http.ResponseWriter that includes the request context
type responseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (rw *responseWriter) Context() context.Context {
	return rw.req.Context()
}

func (p *Portal) handleAdminUpdateOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	logRequest(r, "ADMIN", "Attempting to update order:", orderID)

	// In a real application, you would parse the form and update the order
	// For this example, we'll just log it and redirect
	logRequest(r, "ADMIN", "Order update logic would go here for order:", orderID)

	http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
}

func (p *Portal) handleAdminEditOrderForm(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	logRequest(r, "ADMIN", "Showing edit form for order:", orderID)

	// In a real application, you would fetch the order details
	// For this example, we'll just render a placeholder
	p.renderTemplate(w, r, "admin-edit-order.html", "Edit Order", map[string]interface{}{
		"OrderID": orderID,
	})
}

func (p *Portal) handleJobOutputs(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	logRequest(r, "JOBS", "Viewing outputs for job:", jobID)

	s3Client, err := s3.NewClient(p.config)
	if err != nil {
		logRequest(r, "JOBS", "Failed to create S3 client for job", jobID, ":", err)
		http.Error(w, "Could not access file storage.", http.StatusInternalServerError)
		return
	}

	files, err := s3Client.ListJobFiles(jobID)
	if err != nil {
		logRequest(r, "JOBS", "Failed to list files for job", jobID, ":", err)
		http.Error(w, "Could not retrieve job files.", http.StatusInternalServerError)
		return
	}

	p.renderTemplate(w, r, "job-outputs.html", "Job Outputs", map[string]interface{}{
		"JobID": jobID,
		"Files": files,
	})
}
