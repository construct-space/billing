package models

import "time"

type Session struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Token     string    `gorm:"size:128;uniqueIndex;not null" json:"-"`
	UserID    string    `gorm:"size:36;not null" json:"user_id"`
	UserAgent *string   `gorm:"type:text" json:"user_agent"`
	IPAddress *string   `gorm:"size:255" json:"ip_address"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
// TODO(backfill): The sessions.user_id column was previously BIGINT (numeric).
// AutoMigrate will add a new VARCHAR(36) column. Run the following manually:
//   ALTER TABLE sessions ADD COLUMN user_id_uuid VARCHAR(36);
//   UPDATE sessions s JOIN users u ON s.user_id = u.id SET s.user_id_uuid = u.uuid;
//   ALTER TABLE sessions DROP COLUMN user_id;
//   ALTER TABLE sessions RENAME COLUMN user_id_uuid TO user_id;

func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

type Plan struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:100;not null" json:"name"`
	Slug            string    `gorm:"size:50;uniqueIndex;not null" json:"slug"`
	PriceCents      int       `gorm:"not null;default:0" json:"price_cents"`
	Currency        string    `gorm:"size:10;default:EUR" json:"currency"`
	BillingInterval string    `gorm:"size:20;default:monthly" json:"billing_interval"`
	CreditsIncluded int       `gorm:"default:0" json:"credits_included"`
	Features        *string   `gorm:"type:text" json:"features,omitempty"`
	PolarProductID  *string   `gorm:"size:100" json:"polar_product_id,omitempty"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	SortOrder       int       `gorm:"default:0" json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TODO(backfill): subscriptions.user_id was previously BIGINT. See sessions TODO above for backfill steps.
type Subscription struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	UserID              string     `gorm:"size:36;not null;index" json:"user_id"`
	PlanID              uint       `gorm:"not null;index" json:"plan_id"`
	Plan                Plan       `gorm:"foreignKey:PlanID" json:"plan"`
	Status              string     `gorm:"size:50;not null;default:active" json:"status"`
	PolarSubscriptionID *string    `gorm:"size:100" json:"polar_subscription_id,omitempty"`
	PolarCustomerID     *string    `gorm:"size:100" json:"polar_customer_id,omitempty"`
	CurrentPeriodStart  *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd    *time.Time `json:"current_period_end,omitempty"`
	CanceledAt          *time.Time `json:"canceled_at,omitempty"`
	CancelAtPeriodEnd   bool       `gorm:"default:false" json:"cancel_at_period_end"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TODO(backfill): credit_balances.user_id was previously BIGINT. See sessions TODO above for backfill steps.
type CreditBalance struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"size:36;uniqueIndex;not null" json:"user_id"`
	Balance   int       `gorm:"not null;default:0" json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TODO(backfill): credit_transactions.user_id was previously BIGINT. See sessions TODO above for backfill steps.
type CreditTransaction struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       string    `gorm:"size:36;not null;index" json:"user_id"`
	Amount       int       `gorm:"not null" json:"amount"`
	Type         string    `gorm:"size:50;not null" json:"type"`
	Description  string    `gorm:"size:500" json:"description"`
	Reference    string    `gorm:"size:255" json:"reference"`
	OrderID      *uint     `gorm:"index" json:"order_id,omitempty"`
	BalanceAfter int       `gorm:"not null" json:"balance_after"`
	CreatedAt    time.Time `json:"created_at"`
}

// TODO(backfill): orders.user_id was previously BIGINT. See sessions TODO above for backfill steps.
type Order struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         string    `gorm:"size:36;not null;index" json:"user_id"`
	PolarOrderID   string    `gorm:"size:100;uniqueIndex" json:"polar_order_id"`
	PolarProductID string    `gorm:"size:100" json:"polar_product_id"`
	ProductType    string    `gorm:"size:50;not null" json:"product_type"`
	ProductName    string    `gorm:"size:255" json:"product_name"`
	Amount         int       `gorm:"not null" json:"amount"`
	Currency       string    `gorm:"size:10;default:EUR" json:"currency"`
	Status         string    `gorm:"size:50;not null" json:"status"`
	Metadata       *string   `gorm:"type:text" json:"metadata,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
