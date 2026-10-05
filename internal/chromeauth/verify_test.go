package chromeauth

import "testing"

func TestVerifiedPageIdentity(t *testing.T) {
	for _, html := range []string{
		`<script>window.WIZ_global_data={"FdrFJe":"123456789","oPEP7c":"Account@Example.Test"};</script>`,
		`<script nonce="fixture">window.WIZ_global_data = {"oPEP7c":"Account\u0040Example.Test"};</script>`,
		`<script type="application/json" id="random-id-a">{"qwAQke":"MakerSuiteHttp","WIu0Nc":"public-fixture-key","FdrFJe":"123456789","oPEP7c":"Account\u0040Example.Test"}</script>`,
		`<script id="random-id-b" type="application/json">{"qwAQke":"MakerSuiteUi","WIu0Nc":"public-fixture-key","FdrFJe":"987654321","oPEP7c":"Account@Example.Test"}</script>`,
	} {
		if got := verifiedPageEmail(html); got != "account@example.test" {
			t.Fatalf("email=%q", got)
		}
	}
	for _, html := range []string{
		`prompt mentions unrelated@example.test`,
		`<script>window.WIZ_global_data={"FdrFJe":"email@example.test"};</script>`,
		`<script>window.WIZ_global_data={"oPEP7c":"Name <email@example.test>"};</script>`,
		`<p>window.WIZ_global_data={"oPEP7c":"unrelated@example.test"};</p>`,
		`<script>const prompt={"oPEP7c":"unrelated@example.test"};</script>`,
		`<script>window.WIZ_global_data={"nested":{"oPEP7c":"unrelated@example.test"}};</script>`,
		`<script type="application/json">{"oPEP7c":"unrelated@example.test"}</script>`,
		`<script type="application/json">{"qwAQke":"PromptData","WIu0Nc":"public-fixture-key","FdrFJe":"123456789","oPEP7c":"unrelated@example.test"}</script>`,
	} {
		if got := verifiedPageEmail(html); got != "" {
			t.Fatal("untrusted email accepted")
		}
	}
}
