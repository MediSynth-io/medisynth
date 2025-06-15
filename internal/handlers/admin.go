package handlers

import (
	"net/http"

	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/gin-gonic/gin"
)

// AdminUsers handles the user management page
func (h *Handler) AdminUsers(c *gin.Context) {
	users, err := h.db.GetAllUsers()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to fetch users",
		})
		return
	}

	c.HTML(http.StatusOK, "admin-users.html", gin.H{
		"Data": gin.H{
			"Users": users,
		},
	})
}

// UpdateUserState handles updating a user's state
func (h *Handler) UpdateUserState(c *gin.Context) {
	userID := c.Param("id")
	state := c.PostForm("state")

	// Validate state
	validStates := map[string]bool{
		"active":  true,
		"on_hold": true,
		"paid":    true,
		"free":    true,
		"deleted": true,
	}

	if !validStates[state] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state"})
		return
	}

	// Update user state
	err := h.db.UpdateUserState(userID, models.AccountState(state))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user state"})
		return
	}

	// Redirect back to users page
	c.Redirect(http.StatusSeeOther, "/admin/users")
}

// ForcePasswordReset forces a user to reset their password on next login
func (h *Handler) ForcePasswordReset(c *gin.Context) {
	userID := c.Param("id")

	err := h.db.ForcePasswordReset(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to force password reset"})
		return
	}

	// Redirect back to users page
	c.Redirect(http.StatusSeeOther, "/admin/users")
}

// AdminDashboard handles the admin dashboard page
func (h *Handler) AdminDashboard(c *gin.Context) {
	// Get user statistics
	users, err := h.db.GetAllUsers()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to fetch user statistics",
		})
		return
	}

	// Calculate user state statistics
	var (
		totalUsers  = len(users)
		activeUsers = 0
		onHoldUsers = 0
		paidUsers   = 0
		userStates  = make(map[string]int)
	)

	for _, user := range users {
		userStates[string(user.State)]++
		switch user.State {
		case models.AccountStateActive:
			activeUsers++
		case models.AccountStateOnHold:
			onHoldUsers++
		case models.AccountStatePaid:
			paidUsers++
		}
	}

	// Convert user states to percentage
	var userStateStats []struct {
		State      string
		Count      int
		Percentage float64
	}
	for state, count := range userStates {
		percentage := 0.0
		if totalUsers > 0 {
			percentage = float64(count) / float64(totalUsers) * 100
		}
		userStateStats = append(userStateStats, struct {
			State      string
			Count      int
			Percentage float64
		}{
			State:      state,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Get recent orders
	recentOrders, err := h.db.GetRecentOrders(5)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to fetch recent orders",
		})
		return
	}

	c.HTML(http.StatusOK, "admin-dashboard.html", gin.H{
		"Data": gin.H{
			"TotalUsers":   totalUsers,
			"ActiveUsers":  activeUsers,
			"OnHoldUsers":  onHoldUsers,
			"PaidUsers":    paidUsers,
			"UserStates":   userStateStats,
			"RecentOrders": recentOrders,
		},
	})
}
