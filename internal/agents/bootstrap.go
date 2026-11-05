package agents

import (
	"context"
	"slices"

	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// SyncLadder loads wards and officials from the pilot definition. Officials
// are matched by name and rung; phone numbers are encrypted on the way in.
func (e *Engine) SyncLadder(ctx context.Context, l *config.Ladder) error {
	return e.Store.Tx(ctx, func(tx *store.Store) error {
		for _, wd := range l.Wards {
			if err := tx.UpsertWard(ctx, wd.ToWard()); err != nil {
				return err
			}
		}
		existing, err := tx.ListOfficials(ctx, "")
		if err != nil {
			return err
		}
		for _, od := range l.Officials {
			o := domain.Official{Name: od.Name, Role: od.Role, Department: od.Department, WardIDs: od.Wards, Email: od.Email,
				LadderLevel: domain.LadderLevel(od.Level), GrievanceChannel: od.GrievanceChannel, PreferredChannel: od.PreferredChannel,
				Language: od.Language, Active: true}
			if o.PreferredChannel == "" {
				switch {
				case od.Email != "":
					o.PreferredChannel = "email"
				case od.Phone != "":
					o.PreferredChannel = "whatsapp"
				default:
					o.PreferredChannel = "grievance"
				}
			}
			if od.Phone != "" {
				p, err := secure.NormalizePhone(od.Phone)
				if err != nil {
					return err
				}
				if o.PhoneEnc, err = e.Vault.Encrypt(p); err != nil {
					return err
				}
				o.PhoneHash = e.Vault.HashPhone(p)
			}
			idx := slices.IndexFunc(existing, func(x domain.Official) bool { return x.Name == o.Name && x.LadderLevel == o.LadderLevel })
			if idx >= 0 {
				o.ID = existing[idx].ID
				if err := tx.UpdateOfficial(ctx, o); err != nil {
					return err
				}
				continue
			}
			if err := tx.CreateOfficial(ctx, &o); err != nil {
				return err
			}
		}
		return nil
	})
}
