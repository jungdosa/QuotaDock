package codex

import (
	"encoding/json"
	"github.com/jungdosa/QuotaDock/internal/model"
	"sort"
	"strconv"
	"strings"
	"time"
)

type rateWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	WindowDurationMins *int     `json:"windowDurationMins"`
	ResetsAt           *int64   `json:"resetsAt"`
}

type creditState struct {
	Balance    json.RawMessage `json:"balance"`
	Unlimited  *bool           `json:"unlimited"`
	HasCredits *bool           `json:"hasCredits"`
}

type resetCreditState struct {
	AvailableCount int `json:"availableCount"`
}

type rateSnapshot struct {
	Primary   *rateWindow  `json:"primary"`
	Secondary *rateWindow  `json:"secondary"`
	Credits   *creditState `json:"credits"`
	LimitID   string       `json:"limitId"`
	LimitName string       `json:"limitName"`
	PlanType  string       `json:"planType"`
}

type rateBucket struct {
	Snapshot rateSnapshot
	Legacy   *rateWindow
}

func (b *rateBucket) UnmarshalJSON(raw []byte) error {
	var probe struct {
		UsedPercent json.RawMessage `json:"usedPercent"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	if len(probe.UsedPercent) > 0 {
		var window rateWindow
		if err := json.Unmarshal(raw, &window); err != nil {
			return err
		}
		b.Legacy = &window
		return nil
	}
	return json.Unmarshal(raw, &b.Snapshot)
}

type rateEnvelope struct {
	RateLimitResetCredits resetCreditState       `json:"rateLimitResetCredits"`
	RateLimits            rateSnapshot           `json:"rateLimits"`
	RateLimitsByLimitID   map[string]*rateBucket `json:"rateLimitsByLimitId"`
	Credits               *creditState           `json:"credits"`
}

func (p *Provider) ApplyRateLimitsUpdated(raw json.RawMessage) error {
	var update rateEnvelope
	if err := json.Unmarshal(raw, &update); err != nil {
		return model.SafeError{Code: model.ErrInvalidResponse, Key: "error.invalid_response"}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limits.RateLimits = mergeSnapshot(p.limits.RateLimits, update.RateLimits)
	if normalized := model.NormalizePlan(model.ProviderCodex, update.RateLimits.PlanType); normalized != model.PlanUnknown {
		p.plan = normalized
	}
	if p.limits.RateLimitsByLimitID == nil {
		p.limits.RateLimitsByLimitID = make(map[string]*rateBucket)
	}
	if update.RateLimits.LimitID != "" {
		current := p.limits.RateLimitsByLimitID[update.RateLimits.LimitID]
		if current == nil {
			current = &rateBucket{}
		}
		current.Snapshot = mergeSnapshot(current.Snapshot, update.RateLimits)
		p.limits.RateLimitsByLimitID[update.RateLimits.LimitID] = current
	}
	for id, bucket := range update.RateLimitsByLimitID {
		if bucket == nil {
			continue
		}
		current := p.limits.RateLimitsByLimitID[id]
		if current == nil {
			current = &rateBucket{}
		}
		if bucket.Legacy != nil {
			current.Legacy = mergeWindow(current.Legacy, bucket.Legacy)
		} else {
			current.Snapshot = mergeSnapshot(current.Snapshot, bucket.Snapshot)
		}
		p.limits.RateLimitsByLimitID[id] = current
	}
	if update.Credits != nil {
		p.limits.Credits = mergeCredits(p.limits.Credits, update.Credits)
	}
	return nil
}

func mergeSnapshot(current, update rateSnapshot) rateSnapshot {
	current.Primary = mergeWindow(current.Primary, update.Primary)
	current.Secondary = mergeWindow(current.Secondary, update.Secondary)
	if update.Credits != nil {
		current.Credits = mergeCredits(current.Credits, update.Credits)
	}
	if update.LimitID != "" {
		current.LimitID = update.LimitID
	}
	if update.LimitName != "" {
		current.LimitName = update.LimitName
	}
	if update.PlanType != "" {
		current.PlanType = update.PlanType
	}
	return current
}

func mergeWindow(current, update *rateWindow) *rateWindow {
	if update == nil {
		return current
	}
	if current == nil {
		current = &rateWindow{}
	}
	copy := *current
	if update.UsedPercent != nil {
		copy.UsedPercent = update.UsedPercent
	}
	if update.WindowDurationMins != nil {
		copy.WindowDurationMins = update.WindowDurationMins
	}
	if update.ResetsAt != nil {
		copy.ResetsAt = update.ResetsAt
	}
	return &copy
}

func mergeCredits(current, update *creditState) *creditState {
	if update == nil {
		return current
	}
	if current == nil {
		current = &creditState{}
	}
	copy := *current
	if len(update.Balance) > 0 && string(update.Balance) != "null" {
		copy.Balance = append(json.RawMessage(nil), update.Balance...)
	}
	if update.Unlimited != nil {
		copy.Unlimited = update.Unlimited
	}
	if update.HasCredits != nil {
		copy.HasCredits = update.HasCredits
	}
	return &copy
}

func snapshotFrom(plan model.Plan, envelope rateEnvelope, fetchedAt time.Time) model.UsageSnapshot {
	snapshot := model.UsageSnapshot{Provider: model.ProviderCodex, Plan: plan, FetchedAt: fetchedAt.UTC()}
	appendWindow := func(limits *[]model.UsageLimit, id, label string, window *rateWindow) {
		if window == nil || window.UsedPercent == nil {
			return
		}
		used, remaining := model.PercentPair(*window.UsedPercent, false)
		limit := model.UsageLimit{ID: id, UsedPercent: used, RemainingPercent: remaining}
		if window.WindowDurationMins != nil {
			limit.WindowMinutes = *window.WindowDurationMins
		}
		if label == "" {
			limit.Label = model.UsageWindowLabel(limit.WindowMinutes)
		} else {
			limit.Label = label
		}
		if window.ResetsAt != nil {
			limit.ResetsAt = time.Unix(*window.ResetsAt, 0).UTC()
		}
		*limits = append(*limits, limit)
	}
	appendSnapshot := func(limits *[]model.UsageLimit, idPrefix, label string, rate rateSnapshot) {
		primaryID, secondaryID := "primary", "secondary"
		if idPrefix != "" {
			primaryID = idPrefix + ":primary"
			secondaryID = idPrefix + ":secondary"
		}
		appendWindow(limits, primaryID, label, rate.Primary)
		appendWindow(limits, secondaryID, label, rate.Secondary)
	}
	sortByWindow := func(limits []model.UsageLimit) {
		sort.SliceStable(limits, func(i, j int) bool {
			left, right := limits[i].WindowMinutes, limits[j].WindowMinutes
			if left <= 0 {
				return false
			}
			if right <= 0 {
				return true
			}
			return left < right
		})
	}

	ids := make([]string, 0, len(envelope.RateLimitsByLimitID))
	for id := range envelope.RateLimitsByLimitID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	type namedBucket struct {
		id       string
		snapshot rateSnapshot
	}
	generalLimits := make([]model.UsageLimit, 0, 2+len(ids)*2)
	namedBuckets := make([]namedBucket, 0, len(ids)+1)
	if strings.TrimSpace(envelope.RateLimits.LimitName) == "" {
		appendSnapshot(&generalLimits, "", "", envelope.RateLimits)
	} else {
		namedBuckets = append(namedBuckets, namedBucket{id: envelope.RateLimits.LimitID, snapshot: envelope.RateLimits})
	}
	for _, id := range ids {
		bucket := envelope.RateLimitsByLimitID[id]
		if bucket == nil {
			continue
		}
		if bucket.Legacy != nil {
			appendWindow(&generalLimits, id, "", bucket.Legacy)
			continue
		}
		if strings.TrimSpace(bucket.Snapshot.LimitName) == "" {
			appendSnapshot(&generalLimits, id, "", bucket.Snapshot)
			continue
		}
		namedBuckets = append(namedBuckets, namedBucket{id: id, snapshot: bucket.Snapshot})
	}
	// The unnamed top-level and nested buckets are duplicate views of the same
	// general allowance. Keep their historical duration-based merge, but never
	// merge a named model bucket into that allowance or into another model.
	if len(generalLimits) > 1 {
		seen := make(map[int]int, len(generalLimits))
		deduped := make([]model.UsageLimit, 0, len(generalLimits))
		for _, limit := range generalLimits {
			if limit.WindowMinutes > 0 {
				if idx, ok := seen[limit.WindowMinutes]; ok {
					if limit.UsedPercent > deduped[idx].UsedPercent {
						deduped[idx] = limit
					}
					continue
				}
				seen[limit.WindowMinutes] = len(deduped)
			}
			deduped = append(deduped, limit)
		}
		generalLimits = deduped
	}
	sortByWindow(generalLimits)
	snapshot.Limits = append(snapshot.Limits, generalLimits...)

	sort.SliceStable(namedBuckets, func(i, j int) bool {
		return namedBuckets[i].id < namedBuckets[j].id
	})
	// The top-level rateLimits mirrors one of the nested buckets. When that
	// mirror is a named model bucket it would otherwise appear twice under the
	// same id; keep the first occurrence only.
	seenNamed := make(map[string]bool, len(namedBuckets))
	for _, bucket := range namedBuckets {
		if seenNamed[bucket.id] {
			continue
		}
		seenNamed[bucket.id] = true
		limits := make([]model.UsageLimit, 0, 2)
		appendSnapshot(&limits, bucket.id, shortLimitName(bucket.snapshot.LimitName), bucket.snapshot)
		sortByWindow(limits)
		snapshot.Limits = append(snapshot.Limits, limits...)
	}
	credits := envelope.Credits
	if envelope.RateLimits.Credits != nil {
		credits = envelope.RateLimits.Credits
	}
	if credits == nil {
		for _, id := range ids {
			bucket := envelope.RateLimitsByLimitID[id]
			if bucket != nil && bucket.Snapshot.Credits != nil {
				credits = bucket.Snapshot.Credits
				break
			}
		}
	}
	resetCredits := envelope.RateLimitResetCredits.AvailableCount
	if resetCredits < 0 {
		resetCredits = 0
	}
	if credits != nil || resetCredits > 0 {
		value := &model.Credits{ResetCredits: resetCredits}
		if credits != nil {
			if balance, ok := parseBalance(credits.Balance); ok {
				value.Balance = balance
			}
			if credits.Unlimited != nil {
				value.Unlimited = *credits.Unlimited
			}
			if credits.HasCredits != nil {
				value.HasCredits = *credits.HasCredits
			}
		}
		snapshot.Credits = value
	}
	return snapshot
}

func shortLimitName(limitName string) string {
	name := strings.TrimSpace(limitName)
	if idx := strings.LastIndex(name, "-"); idx >= 0 {
		if token := strings.TrimSpace(name[idx+1:]); token != "" {
			return token
		}
	}
	return name
}

func parseBalance(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return number, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil
}

var _ model.Provider = (*Provider)(nil)
