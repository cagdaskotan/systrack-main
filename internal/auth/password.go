package auth

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

type User struct {
	ID            int        `json:"id"`
	Email         string     `json:"email"`
	PassHash      string     `json:"-"`
	Role          string     `json:"role"`
	MaxTargets    int        `json:"max_targets"`
	IPQueryLimit  int        `json:"ip_query_limit"`
	IsActive      bool       `json:"is_active"`
	LicenseExpiry *time.Time `json:"license_expiry"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PublicUser struct for API responses (without sensitive data)
type PublicUser struct {
	ID            int        `json:"id"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	MaxTargets    int        `json:"max_targets"`
	IPQueryLimit  int        `json:"ip_query_limit"`
	IsActive      bool       `json:"is_active"`
	LicenseExpiry *time.Time `json:"license_expiry"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func CreateUser(db *sql.DB, email, password, role string, maxTargets, ipQueryLimit int, licenseExpiry *time.Time) (*User, error) {
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Set default max targets based on role
	if maxTargets <= 0 {
		switch role {
		case "admin":
			maxTargets = 1000 // Unlimited for admin
		case "user":
			maxTargets = 50
		case "viewer":
			maxTargets = 10
		default:
			maxTargets = 10
		}
	}

	// Set default IP query limit based on role
	if ipQueryLimit <= 0 {
		switch role {
		case "admin":
			ipQueryLimit = 100
		case "user":
			ipQueryLimit = 20
		case "viewer":
			ipQueryLimit = 5
		default:
			ipQueryLimit = 10
		}
	}

	now := time.Now()
	query := `INSERT INTO users (email, pass_hash, role, max_targets, ip_query_limit, is_active, license_expiry, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := db.Exec(query, email, hashedPassword, role, maxTargets, ipQueryLimit, 1, licenseExpiry, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	return &User{
		ID:            int(id),
		Email:         email,
		PassHash:      hashedPassword,
		Role:          role,
		MaxTargets:    maxTargets,
		IsActive:      true,
		LicenseExpiry: licenseExpiry,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	query := `SELECT id, email, pass_hash, role, max_targets, ip_query_limit, is_active, license_expiry, last_login_at, created_at, updated_at FROM users WHERE email = ?`
	row := db.QueryRow(query, email)

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.PassHash, &user.Role, &user.MaxTargets, &user.IPQueryLimit, &user.IsActive, &user.LicenseExpiry, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

func GetUserByID(db *sql.DB, id int) (*User, error) {
	query := `SELECT id, email, pass_hash, role, max_targets, ip_query_limit, is_active, license_expiry, last_login_at, created_at, updated_at FROM users WHERE id = ?`
	row := db.QueryRow(query, id)

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.PassHash, &user.Role, &user.MaxTargets, &user.IPQueryLimit, &user.IsActive, &user.LicenseExpiry, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

func AuthenticateUser(db *sql.DB, email, password string) (*User, error) {
	user, err := GetUserByEmail(db, email)
	if err != nil {
		return nil, err
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is disabled")
	}

	// Check license expiry
	if user.LicenseExpiry != nil && user.LicenseExpiry.Before(time.Now()) {
		// Auto-deactivate expired users
		now := time.Now()
		query := `UPDATE users SET is_active = FALSE, updated_at = ? WHERE id = ?`
		_, err = db.Exec(query, now, user.ID)
		if err != nil {
			fmt.Printf("Failed to deactivate expired user: %v\n", err)
		}
		return nil, fmt.Errorf("user license has expired")
	}

	if !CheckPasswordHash(password, user.PassHash) {
		return nil, fmt.Errorf("invalid password")
	}

	// Update last login time
	now := time.Now()
	query := `UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?`
	_, err = db.Exec(query, now, now, user.ID)
	if err != nil {
		// Log error but don't fail authentication
		fmt.Printf("Failed to update last login time: %v\n", err)
	}

	user.LastLoginAt = &now
	user.UpdatedAt = now

	return user, nil
}

func UserExists(db *sql.DB, email string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE email = ?`
	var count int
	err := db.QueryRow(query, email).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateUser updates user information
func UpdateUser(db *sql.DB, id int, email, role string, maxTargets, ipQueryLimit int, isActive bool, licenseExpiry *time.Time) error {
	now := time.Now()
	query := `UPDATE users SET email = ?, role = ?, max_targets = ?, ip_query_limit = ?, is_active = ?, license_expiry = ?, updated_at = ? WHERE id = ?`

	// Convert boolean to int for MySQL
	var isActiveInt int
	if isActive {
		isActiveInt = 1
	}

	// Format license expiry for MySQL
	var licenseExpiryFormatted interface{}
	if licenseExpiry != nil {
		// Convert to MySQL datetime format
		licenseExpiryFormatted = licenseExpiry.Format("2006-01-02 15:04:05")
	}

	_, err := db.Exec(query, email, role, maxTargets, ipQueryLimit, isActiveInt, licenseExpiryFormatted, now, id)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// GetAllUsers returns all users
func GetAllUsers(db *sql.DB) ([]User, error) {
	query := `SELECT id, email, role, max_targets, ip_query_limit, is_active, license_expiry, last_login_at, created_at, updated_at FROM users ORDER BY created_at DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.MaxTargets, &user.IPQueryLimit, &user.IsActive, &user.LicenseExpiry, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// DeleteUser deletes a user
func DeleteUser(db *sql.DB, id int) error {
	query := `DELETE FROM users WHERE id = ?`
	_, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// GetUserTargetCount returns the number of targets for a user
func GetUserTargetCount(db *sql.DB, userID int) (int, error) {
	query := `SELECT COUNT(*) FROM targets WHERE user_id = ?`
	var count int
	err := db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get target count: %w", err)
	}
	return count, nil
}

// CheckLicenseExpiry checks if user's license has expired
func CheckLicenseExpiry(db *sql.DB, userID int) (bool, error) {
	user, err := GetUserByID(db, userID)
	if err != nil {
		return false, err
	}

	if user.LicenseExpiry == nil {
		return false, nil // No expiry date set
	}

	return user.LicenseExpiry.Before(time.Now()), nil
}

// ExtendLicense extends user's license expiry date
func ExtendLicense(db *sql.DB, userID int, newExpiryDate time.Time) error {
	now := time.Now()
	query := `UPDATE users SET license_expiry = ?, is_active = TRUE, updated_at = ? WHERE id = ?`
	_, err := db.Exec(query, newExpiryDate, now, userID)
	if err != nil {
		return fmt.Errorf("failed to extend license: %w", err)
	}
	return nil
}
