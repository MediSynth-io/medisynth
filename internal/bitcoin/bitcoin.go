package bitcoin

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/skip2/go-qrcode"
)

// MempoolTransaction represents a transaction from Mempool.space API
type MempoolTransaction struct {
	TxID   string `json:"txid"`
	Status struct {
		Confirmed   bool `json:"confirmed"`
		BlockHeight int  `json:"block_height"`
	} `json:"status"`
	Vout []struct {
		ScriptPubkeyAddress string `json:"scriptpubkey_address"`
		Value               int64  `json:"value"` // in satoshis
	} `json:"vout"`
	Fee           int64 `json:"fee"`
	Confirmations int   `json:"confirmations,omitempty"`
}

// BitcoinService handles Bitcoin transaction verification
type BitcoinService struct {
	httpClient *http.Client
	apiURL     string
}

// NewBitcoinService creates a new Bitcoin service with proper TLS configuration
func NewBitcoinService() *BitcoinService {
	// Create HTTP client with proper TLS configuration for Kubernetes
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Keep security but handle cert issues
		},
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}

	return &BitcoinService{
		httpClient: client,
		apiURL:     "https://mempool.space/api",
	}
}

// CheckAddressTransactions checks for transactions to the given Bitcoin address
func (s *BitcoinService) CheckAddressTransactions(address string) ([]MempoolTransaction, error) {
	url := fmt.Sprintf("%s/address/%s/txs", s.apiURL, address)

	log.Printf("[BITCOIN] Checking transactions for address: %s", address)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var transactions []MempoolTransaction
	if err := json.NewDecoder(resp.Body).Decode(&transactions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	log.Printf("[BITCOIN] Found %d transactions for address %s", len(transactions), address)
	return transactions, nil
}

// GetTransactionDetails gets detailed information about a specific transaction
func (s *BitcoinService) GetTransactionDetails(txid string) (*MempoolTransaction, error) {
	url := fmt.Sprintf("%s/tx/%s", s.apiURL, txid)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transaction details: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var tx MempoolTransaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %v", err)
	}

	return &tx, nil
}

// SatoshisToBTC converts satoshis to BTC
func SatoshisToBTC(satoshis int64) float64 {
	return float64(satoshis) / 100000000.0
}

// BTCToSatoshis converts BTC to satoshis
func BTCToSatoshis(btc float64) int64 {
	return int64(btc * 100000000.0)
}

// VerifyPayments checks all pending orders for Bitcoin payments
func (s *BitcoinService) VerifyPayments(btcAddress string) error {
	log.Printf("[BITCOIN] Starting payment verification for address: %s", btcAddress)

	// Get all pending orders
	pendingOrders, err := database.GetPendingOrders()
	if err != nil {
		return fmt.Errorf("failed to get pending orders: %v", err)
	}

	if len(pendingOrders) == 0 {
		log.Printf("[BITCOIN] No pending orders to verify")
		return nil
	}

	log.Printf("[BITCOIN] Found %d pending orders to verify", len(pendingOrders))

	// Get recent transactions for the address
	transactions, err := s.CheckAddressTransactions(btcAddress)
	if err != nil {
		return fmt.Errorf("failed to check address transactions: %v", err)
	}

	// Check each pending order against the transactions
	for _, order := range pendingOrders {
		if err := s.verifyOrderPayment(order, transactions, btcAddress); err != nil {
			log.Printf("[BITCOIN] Error verifying order %s: %v", order.OrderNumber, err)
			continue
		}
	}

	return nil
}

// verifyOrderPayment checks if a specific order has been paid
func (s *BitcoinService) verifyOrderPayment(order *models.Order, transactions []MempoolTransaction, btcAddress string) error {
	// Handle nil pointer for AmountBTC
	if order.AmountBTC == nil {
		log.Printf("[BITCOIN] Order %s has no BTC amount set, skipping", order.OrderNumber)
		return nil
	}

	expectedAmount := BTCToSatoshis(*order.AmountBTC)

	log.Printf("[BITCOIN] Verifying payment for order %s, expected: %.8f BTC (%d sats)",
		order.OrderNumber, *order.AmountBTC, expectedAmount)

	for _, tx := range transactions {
		// Skip if we've already processed this transaction
		if order.TransactionHash != nil && *order.TransactionHash != "" && *order.TransactionHash == tx.TxID {
			// Update confirmations if needed
			if tx.Status.Confirmed && order.Status != models.OrderStatusConfirmed {
				if err := database.UpdateOrderConfirmations(order.ID, 6); err != nil {
					log.Printf("[BITCOIN] Failed to update confirmations for order %s: %v", order.OrderNumber, err)
				}
			}
			continue
		}

		// Check if this transaction sends to our address with the right amount
		for _, output := range tx.Vout {
			if output.ScriptPubkeyAddress == btcAddress && output.Value >= expectedAmount {
				log.Printf("[BITCOIN] Payment found! TxID: %s, Amount: %d sats", tx.TxID, output.Value)

				// Determine confirmations
				confirmations := 0
				if tx.Status.Confirmed {
					confirmations = 6 // Assume confirmed transactions have enough confirmations
				}

				// Update order with payment information
				if err := database.UpdateOrderPayment(order.ID, tx.TxID, SatoshisToBTC(output.Value), confirmations); err != nil {
					return fmt.Errorf("failed to update order payment: %v", err)
				}

				// Create payment record
				payment := &models.Payment{
					OrderID:         order.ID,
					TransactionHash: tx.TxID,
					AmountBTC:       SatoshisToBTC(output.Value),
					Confirmations:   confirmations,
					Status:          models.PaymentStatusDetected,
					DetectedAt:      time.Now(),
				}

				if tx.Status.Confirmed {
					payment.Status = models.PaymentStatusConfirmed
					confirmedAt := time.Now()
					payment.ConfirmedAt = &confirmedAt
				}

				if err := database.CreatePayment(payment); err != nil {
					log.Printf("[BITCOIN] Failed to create payment record: %v", err)
				}

				log.Printf("[BITCOIN] Order %s payment verified successfully", order.OrderNumber)
				return nil
			}
		}
	}

	return nil
}

// StartMonitoring starts the Bitcoin payment monitoring service
func (s *BitcoinService) StartMonitoring(btcAddress string, interval time.Duration) {
	if btcAddress == "" {
		log.Printf("[BITCOIN] No Bitcoin address configured, skipping monitoring")
		return
	}

	log.Printf("[BITCOIN] Starting payment monitoring for address: %s (interval: %v)", btcAddress, interval)

	// Initial check
	if err := s.VerifyPayments(btcAddress); err != nil {
		log.Printf("[BITCOIN] Initial payment verification failed: %v", err)
	}

	// Start periodic monitoring
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := s.VerifyPayments(btcAddress); err != nil {
				log.Printf("[BITCOIN] Payment verification failed: %v", err)
			}
		}
	}()
}

// BitcoinPriceResponse represents the response from a Bitcoin price API
type BitcoinPriceResponse struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
}

// GetBitcoinPrice fetches the current Bitcoin price in USD
func (s *BitcoinService) GetBitcoinPrice() (float64, error) {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"

	resp, err := s.httpClient.Get(url)
	if err != nil {
		log.Printf("[BITCOIN] Warning: Failed to fetch Bitcoin price from API: %v", err)
		log.Printf("[BITCOIN] Using fallback price of $100,000 USD")
		return 100000.0, nil // Fallback price - roughly current BTC price
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[BITCOIN] Warning: Price API returned status %d, using fallback price", resp.StatusCode)
		return 100000.0, nil // Fallback price
	}

	var priceResp BitcoinPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&priceResp); err != nil {
		log.Printf("[BITCOIN] Warning: Failed to decode price response: %v, using fallback price", err)
		return 100000.0, nil // Fallback price
	}

	if priceResp.Bitcoin.USD <= 0 {
		log.Printf("[BITCOIN] Warning: Invalid price from API: %f, using fallback price", priceResp.Bitcoin.USD)
		return 100000.0, nil // Fallback price
	}

	log.Printf("[BITCOIN] Successfully fetched Bitcoin price: $%.2f USD", priceResp.Bitcoin.USD)
	return priceResp.Bitcoin.USD, nil
}

// ConvertUSDToBTC converts USD amount to BTC using current market price
func (s *BitcoinService) ConvertUSDToBTC(usdAmount float64) (float64, error) {
	btcPrice, err := s.GetBitcoinPrice()
	if err != nil {
		return 0, err
	}

	if btcPrice <= 0 {
		return 0, fmt.Errorf("invalid Bitcoin price: %f", btcPrice)
	}

	btcAmount := usdAmount / btcPrice
	log.Printf("[BITCOIN] Converted $%.2f USD to %.8f BTC (price: $%.2f)", usdAmount, btcAmount, btcPrice)

	return btcAmount, nil
}

// GeneratePaymentQR generates a QR code for Bitcoin payment
func (s *BitcoinService) GeneratePaymentQR(address string, amount float64, label string) (string, error) {
	// Create Bitcoin URI
	bitcoinURI := fmt.Sprintf("bitcoin:%s?amount=%.8f&label=%s", address, amount, label)

	// Generate QR code
	qrCode, err := qrcode.Encode(bitcoinURI, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %v", err)
	}

	// Convert to base64 for HTML embedding
	base64QR := base64.StdEncoding.EncodeToString(qrCode)

	log.Printf("[BITCOIN] Generated QR code for payment: %s", bitcoinURI)
	return base64QR, nil
}

// ProcessOrderPayment processes a new order with Bitcoin payment setup
func (s *BitcoinService) ProcessOrderPayment(userID, description string, amountUSD float64, btcAddress string) (*models.Order, error) {
	log.Printf("[BITCOIN] Processing order payment for user %s, amount: $%.2f, to address: %s", userID, amountUSD, btcAddress)

	if btcAddress == "" {
		log.Printf("[BITCOIN] ERROR: Bitcoin address is not configured. Cannot process order.")
		return nil, fmt.Errorf("bitcoin address is not configured on the server")
	}

	// Convert USD to BTC
	amountBTC, err := s.ConvertUSDToBTC(amountUSD)
	if err != nil {
		return nil, fmt.Errorf("failed to convert USD to BTC: %v", err)
	}

	// Create the order first
	order, err := database.CreateOrder(userID, description, amountUSD, btcAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %v", err)
	}

	// Generate QR code for payment
	qrCode, err := s.GeneratePaymentQR(btcAddress, amountBTC, fmt.Sprintf("Payment for %s", order.OrderNumber))
	if err != nil {
		log.Printf("[BITCOIN] Warning: Failed to generate QR code: %v", err)
		// Continue without QR code
	}

	// Update order with BTC amount and QR code
	if err := database.UpdateOrderBitcoinData(order.ID, amountBTC, qrCode); err != nil {
		log.Printf("[BITCOIN] Warning: Failed to update order with Bitcoin data: %v", err)
		// Continue anyway, order is created
	}

	// Refresh order data to get all fields populated
	updatedOrder, err := database.GetOrderByID(order.ID, userID)
	if err != nil {
		log.Printf("[BITCOIN] Warning: Failed to get updated order details for user %s: %v", userID, err)
		return order, nil // Return original order if refetch fails
	}

	log.Printf("[BITCOIN] Order %s processed successfully with %.8f BTC payment", order.OrderNumber, amountBTC)
	return updatedOrder, nil
}
