package dynacat

// DashDen: starter political-lean ratings for the news-coverage widget.
//
// These are DashDen's own rough placements, written to match where outlets are commonly
// placed by public media-bias charts. They are not copied from any one rating service.
// They are a starting point: override or add outlets with the widget's `sources` option.
//
// Keys are the outlet's web domain without "www.". Subdomains match their parent, so
// "edition.cnn.com" uses "cnn.com".

type newsBias int

const (
	newsBiasUnrated newsBias = iota
	newsBiasLeft
	newsBiasLeanLeft
	newsBiasCenter
	newsBiasLeanRight
	newsBiasRight
)

var newsBiasNames = map[string]newsBias{
	"left":       newsBiasLeft,
	"lean-left":  newsBiasLeanLeft,
	"center":     newsBiasCenter,
	"centre":     newsBiasCenter,
	"lean-right": newsBiasLeanRight,
	"right":      newsBiasRight,
}

func (b newsBias) Label() string {
	switch b {
	case newsBiasLeft:
		return "Left"
	case newsBiasLeanLeft:
		return "Lean left"
	case newsBiasCenter:
		return "Center"
	case newsBiasLeanRight:
		return "Lean right"
	case newsBiasRight:
		return "Right"
	}
	return "Unrated"
}

// Class is the CSS modifier used for the colour dot and the coverage bar.
func (b newsBias) Class() string {
	switch b {
	case newsBiasLeft:
		return "left"
	case newsBiasLeanLeft:
		return "lean-left"
	case newsBiasCenter:
		return "center"
	case newsBiasLeanRight:
		return "lean-right"
	case newsBiasRight:
		return "right"
	}
	return "unrated"
}

// side folds the five-point scale into the three coverage buckets: -1 left, 0 center, 1 right.
func (b newsBias) side() int {
	switch b {
	case newsBiasLeft, newsBiasLeanLeft:
		return -1
	case newsBiasLeanRight, newsBiasRight:
		return 1
	}
	return 0
}

type builtinOutlet struct {
	name       string
	bias       newsBias
	factuality string
}

const (
	nbL  = newsBiasLeft
	nbLL = newsBiasLeanLeft
	nbC  = newsBiasCenter
	nbLR = newsBiasLeanRight
	nbR  = newsBiasRight
)

var builtinNewsOutlets = map[string]builtinOutlet{
	// Wire services and international
	"apnews.com":          {"AP News", nbLL, "high"},
	"reuters.com":         {"Reuters", nbC, "high"},
	"afp.com":             {"AFP", nbC, "high"},
	"bbc.com":             {"BBC News", nbC, "high"},
	"bbc.co.uk":           {"BBC News", nbC, "high"},
	"theguardian.com":     {"The Guardian", nbL, "mixed"},
	"independent.co.uk":   {"The Independent", nbLL, "mixed"},
	"telegraph.co.uk":     {"The Telegraph", nbLR, "mixed"},
	"thetimes.co.uk":      {"The Times", nbLR, "high"},
	"dailymail.co.uk":     {"Daily Mail", nbR, "low"},
	"thesun.co.uk":        {"The Sun", nbR, "low"},
	"mirror.co.uk":        {"Daily Mirror", nbLL, "mixed"},
	"economist.com":       {"The Economist", nbLL, "high"},
	"ft.com":              {"Financial Times", nbC, "high"},
	"aljazeera.com":       {"Al Jazeera", nbLL, "mixed"},
	"dw.com":              {"DW", nbC, "high"},
	"france24.com":        {"France 24", nbLL, "high"},
	"euronews.com":        {"Euronews", nbC, "high"},
	"cbc.ca":              {"CBC News", nbLL, "high"},
	"globalnews.ca":       {"Global News", nbC, "high"},
	"nationalpost.com":    {"National Post", nbLR, "mixed"},
	"theglobeandmail.com": {"The Globe and Mail", nbC, "high"},
	"abc.net.au":          {"ABC Australia", nbLL, "high"},
	"smh.com.au":          {"Sydney Morning Herald", nbLL, "high"},
	"news.com.au":         {"news.com.au", nbLR, "mixed"},
	"skynews.com":         {"Sky News", nbC, "high"},
	"rt.com":              {"RT", nbR, "low"},
	"scmp.com":            {"South China Morning Post", nbC, "mixed"},
	"timesofisrael.com":   {"The Times of Israel", nbC, "high"},
	"jpost.com":           {"Jerusalem Post", nbLR, "mixed"},
	"haaretz.com":         {"Haaretz", nbLL, "high"},

	// US broadcast and cable
	"today.com":        {"TODAY", nbLL, "high"},
	"cnn.com":          {"CNN", nbLL, "mixed"},
	"msnbc.com":        {"MSNBC", nbL, "mixed"},
	"nbcnews.com":      {"NBC News", nbLL, "high"},
	"cbsnews.com":      {"CBS News", nbLL, "high"},
	"abcnews.go.com":   {"ABC News", nbLL, "high"},
	"abcnews.com":      {"ABC News", nbLL, "high"},
	"npr.org":          {"NPR", nbLL, "high"},
	"pbs.org":          {"PBS NewsHour", nbC, "high"},
	"foxnews.com":      {"Fox News", nbR, "mixed"},
	"foxbusiness.com":  {"Fox Business", nbR, "mixed"},
	"newsmax.com":      {"Newsmax", nbR, "low"},
	"oann.com":         {"One America News", nbR, "low"},
	"cnbc.com":         {"CNBC", nbC, "high"},
	"bloomberg.com":    {"Bloomberg", nbLL, "high"},
	"c-span.org":       {"C-SPAN", nbC, "high"},
	"democracynow.org": {"Democracy Now!", nbL, "mixed"},

	// US newspapers and magazines
	"nytimes.com":                 {"The New York Times", nbLL, "high"},
	"washingtonpost.com":          {"The Washington Post", nbLL, "high"},
	"wsj.com":                     {"The Wall Street Journal", nbC, "high"},
	"usatoday.com":                {"USA Today", nbLL, "high"},
	"latimes.com":                 {"Los Angeles Times", nbLL, "high"},
	"chicagotribune.com":          {"Chicago Tribune", nbC, "high"},
	"bostonglobe.com":             {"The Boston Globe", nbLL, "high"},
	"nypost.com":                  {"New York Post", nbLR, "mixed"},
	"washingtontimes.com":         {"The Washington Times", nbLR, "mixed"},
	"washingtonexaminer.com":      {"Washington Examiner", nbR, "mixed"},
	"theatlantic.com":             {"The Atlantic", nbL, "high"},
	"newyorker.com":               {"The New Yorker", nbL, "high"},
	"time.com":                    {"TIME", nbLL, "high"},
	"newsweek.com":                {"Newsweek", nbC, "mixed"},
	"forbes.com":                  {"Forbes", nbC, "mixed"},
	"businessinsider.com":         {"Business Insider", nbLL, "mixed"},
	"politico.com":                {"Politico", nbLL, "high"},
	"thehill.com":                 {"The Hill", nbC, "high"},
	"axios.com":                   {"Axios", nbLL, "high"},
	"semafor.com":                 {"Semafor", nbC, "high"},
	"vox.com":                     {"Vox", nbL, "mixed"},
	"slate.com":                   {"Slate", nbL, "mixed"},
	"salon.com":                   {"Salon", nbL, "mixed"},
	"motherjones.com":             {"Mother Jones", nbL, "mixed"},
	"thenation.com":               {"The Nation", nbL, "mixed"},
	"newrepublic.com":             {"The New Republic", nbL, "mixed"},
	"huffpost.com":                {"HuffPost", nbL, "mixed"},
	"thedailybeast.com":           {"The Daily Beast", nbL, "mixed"},
	"rollingstone.com":            {"Rolling Stone", nbL, "mixed"},
	"theintercept.com":            {"The Intercept", nbL, "mixed"},
	"jacobin.com":                 {"Jacobin", nbL, "mixed"},
	"commondreams.org":            {"Common Dreams", nbL, "mixed"},
	"rawstory.com":                {"Raw Story", nbL, "mixed"},
	"alternet.org":                {"AlterNet", nbL, "low"},
	"propublica.org":              {"ProPublica", nbLL, "high"},
	"theconversation.com":         {"The Conversation", nbLL, "high"},
	"vanityfair.com":              {"Vanity Fair", nbL, "mixed"},
	"buzzfeednews.com":            {"BuzzFeed News", nbL, "mixed"},
	"insider.com":                 {"Insider", nbLL, "mixed"},
	"yahoo.com":                   {"Yahoo News", nbLL, "mixed"},
	"theweek.com":                 {"The Week", nbC, "high"},
	"csmonitor.com":               {"Christian Science Monitor", nbC, "high"},
	"marketwatch.com":             {"MarketWatch", nbC, "high"},
	"barrons.com":                 {"Barron's", nbC, "high"},
	"fortune.com":                 {"Fortune", nbC, "high"},
	"ground.news":                 {"Ground News", nbC, "high"},
	"tangle.media":                {"Tangle", nbC, "high"},
	"realclearpolitics.com":       {"RealClearPolitics", nbLR, "mixed"},
	"reason.com":                  {"Reason", nbLR, "high"},
	"nationalreview.com":          {"National Review", nbR, "mixed"},
	"thedispatch.com":             {"The Dispatch", nbLR, "high"},
	"thefp.com":                   {"The Free Press", nbLR, "mixed"},
	"spectator.org":               {"The American Spectator", nbR, "mixed"},
	"spectator.co.uk":             {"The Spectator", nbLR, "mixed"},
	"theamericanconservative.com": {"The American Conservative", nbR, "mixed"},
	"dailywire.com":               {"The Daily Wire", nbR, "mixed"},
	"dailycaller.com":             {"The Daily Caller", nbR, "mixed"},
	"breitbart.com":               {"Breitbart", nbR, "low"},
	"theblaze.com":                {"TheBlaze", nbR, "mixed"},
	"thefederalist.com":           {"The Federalist", nbR, "mixed"},
	"townhall.com":                {"Townhall", nbR, "mixed"},
	"redstate.com":                {"RedState", nbR, "mixed"},
	"pjmedia.com":                 {"PJ Media", nbR, "mixed"},
	"hotair.com":                  {"Hot Air", nbR, "mixed"},
	"theepochtimes.com":           {"The Epoch Times", nbR, "mixed"},
	"freebeacon.com":              {"Washington Free Beacon", nbR, "mixed"},
	"dailysignal.com":             {"The Daily Signal", nbR, "mixed"},
	"bizpacreview.com":            {"BizPac Review", nbR, "mixed"},
	"justthenews.com":             {"Just The News", nbR, "mixed"},
	"thepostmillennial.com":       {"The Post Millennial", nbR, "mixed"},
	"westernjournal.com":          {"The Western Journal", nbR, "mixed"},
	"americanthinker.com":         {"American Thinker", nbR, "low"},
	"gatewaypundit.com":           {"The Gateway Pundit", nbR, "low"},
	"zerohedge.com":               {"ZeroHedge", nbR, "low"},
	"thegatewaypundit.com":        {"The Gateway Pundit", nbR, "low"},
	"washingtonmonthly.com":       {"Washington Monthly", nbL, "mixed"},
	"thegrio.com":                 {"theGrio", nbL, "mixed"},
	"theroot.com":                 {"The Root", nbL, "mixed"},
	"mediaite.com":                {"Mediaite", nbLL, "mixed"},
	"thebulwark.com":              {"The Bulwark", nbLL, "mixed"},
	"notus.org":                   {"NOTUS", nbC, "high"},
	"19thnews.org":                {"The 19th", nbLL, "high"},
	"texastribune.org":            {"The Texas Tribune", nbC, "high"},
	"calmatters.org":              {"CalMatters", nbC, "high"},
	"factcheck.org":               {"FactCheck.org", nbC, "high"},
	"politifact.com":              {"PolitiFact", nbLL, "high"},
	"snopes.com":                  {"Snopes", nbLL, "high"},

	// Tech and science
	"arstechnica.com":        {"Ars Technica", nbLL, "high"},
	"theverge.com":           {"The Verge", nbLL, "high"},
	"wired.com":              {"WIRED", nbLL, "high"},
	"techcrunch.com":         {"TechCrunch", nbLL, "high"},
	"engadget.com":           {"Engadget", nbLL, "high"},
	"404media.co":            {"404 Media", nbLL, "high"},
	"scientificamerican.com": {"Scientific American", nbLL, "high"},
	"nature.com":             {"Nature", nbC, "high"},
	"science.org":            {"Science", nbC, "high"},
	"newscientist.com":       {"New Scientist", nbLL, "high"},
}

// newsChannelDomains maps video channel names (as newsNameKey gives them) to the outlet's
// domain, for channels whose name differs from the outlet's.
var newsChannelDomains = map[string]string{
	"nbcnewsnow": "nbcnews.com", "meetthepress": "nbcnews.com", "today": "today.com",
	"abcnews": "abcnews.go.com", "goodmorningamerica": "abcnews.go.com",
	"60minutes": "cbsnews.com", "cbsmornings": "cbsnews.com", "facethenation": "cbsnews.com",
	"pbsnewshour": "pbs.org", "frontline": "pbs.org",
	"bbcnews": "bbc.com", "dwnews": "dw.com", "aljazeeraenglish": "aljazeera.com",
	"france24english": "france24.com", "skynews": "skynews.com", "skynewsaustralia": "news.com.au",
	"associatedpress": "apnews.com", "wallstreetjournal": "wsj.com", "wsj": "wsj.com",
	"bloombergtelevision": "bloomberg.com", "bloomberg": "bloomberg.com", "bloomberg_originals": "bloomberg.com",
	"cnbctelevision": "cnbc.com", "cnbcinternational": "cnbc.com", "guardiannews": "theguardian.com",
	"newyorktimes": "nytimes.com", "nytimes": "nytimes.com", "livenowfromfox": "foxnews.com",
	"foxnews": "foxnews.com", "newsmaxtv": "newsmax.com", "newsmax": "newsmax.com",
	"forbesbreakingnews": "forbes.com", "cspan": "c-span.org", "reasontv": "reason.com",
	"economist": "economist.com", "cbcnews": "cbc.ca", "cbcnewsthenational": "cbc.ca",
	"globalnews": "globalnews.ca", "abcnewsindepth": "abc.net.au", "abcnewsaustralia": "abc.net.au",
	"msnbc": "msnbc.com", "msnow": "msnbc.com", "thehill": "thehill.com", "hill": "thehill.com",
	"risingthehill": "thehill.com", "dailycaller": "dailycaller.com", "theblaze": "theblaze.com",
	"blazetv": "theblaze.com", "epochtv": "theepochtimes.com", "ntd": "theepochtimes.com",
	"democracynow": "democracynow.org", "vox": "vox.com",
	"bulwark": "thebulwark.com", "dispatch": "thedispatch.com", "nationalreview": "nationalreview.com",
	"thefreepress": "thefp.com", "freepress": "thefp.com", "politico": "politico.com", "axios": "axios.com",
	"newyorkpost": "nypost.com", "washingtonpost": "washingtonpost.com", "dailywire": "dailywire.com",
}

// builtinNewsChannels rates news shows that live mostly on video sites and have no
// news website of their own. Like the outlets above, these are DashDen's rough
// placements; override them with the widget's sources option.
var builtinNewsChannels = map[string]builtinOutlet{
	"The Young Turks":        {"", nbL, "mixed"},
	"MeidasTouch":            {"", nbL, "mixed"},
	"Brian Tyler Cohen":      {"", nbL, "mixed"},
	"Pod Save America":       {"", nbL, "mixed"},
	"Breaking Points":        {"", nbC, "mixed"},
	"TLDR News":              {"", nbC, "high"},
	"Philip DeFranco":        {"", nbC, "mixed"},
	"Tangle":                 {"", nbC, "high"},
	"Ben Shapiro":            {"", nbR, "mixed"},
	"Tucker Carlson":         {"", nbR, "low"},
	"Tucker Carlson Network": {"", nbR, "low"},
	"Megyn Kelly":            {"", nbR, "mixed"},
	"The Megyn Kelly Show":   {"", nbR, "mixed"},
	"Steven Crowder":         {"", nbR, "low"},
	"Matt Walsh":             {"", nbR, "mixed"},
	"Glenn Beck":             {"", nbR, "mixed"},
}
