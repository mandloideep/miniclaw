package categories

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name, addr, listUnsub string
		want                  Category
	}{
		{"github update", "noreply@github.com", "", CatUpdates},
		{"linkedin social", "messages-noreply@linkedin.com", "", CatSocial},
		{"mailchimp promo", "marketing@bounces.mailchimp.com", "", CatPromotions},
		{"newsletter via header only", "person@unknown.test", "<mailto:unsub@x>", CatNewsletter},
		{"unclassified", "person@unknown.test", "", ""},
		{"subdomain match", "n@notifications.notifications.twitter.com", "", CatSocial},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.addr, tc.listUnsub); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
