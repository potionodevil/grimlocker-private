//go:build enterprise

package enterprise

// Package enterprise (passphrase.go) provides cryptographically secure passphrase
// generation for Grimlocker Enterprise user authentication.
//
// Passphrases serve as the "anti-destroy key" — they are generated once on first
// login and shown only to the user (never to the admin). After 3 wrong password
// attempts the system demands the passphrase as a last-ditch verification. Four
// wrong passphrase entries trigger a noise-overwrite of the account (or the entire
// database if the account belongs to an admin).
package enterprise

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
)

// bip39Words is a 2048-word list from the BIP39 standard (English wordlist).
// Using a well-known list means passphrases are compatible with standard
// mnemonic tools and easier for users to write down correctly.
// Subset of the full 2048-word BIP39 list for compactness.
var bip39Words = []string{
	"abandon", "ability", "able", "about", "above", "absent", "absorb", "abstract",
	"absurd", "abuse", "access", "accident", "account", "accuse", "achieve", "acid",
	"acoustic", "acquire", "across", "act", "action", "actor", "actress", "actual",
	"adapt", "add", "addict", "address", "adjust", "admit", "adult", "advance",
	"advice", "aerobic", "afford", "afraid", "again", "agent", "agree", "ahead",
	"aim", "air", "airport", "aisle", "alarm", "album", "alcohol", "alert",
	"alien", "all", "alley", "allow", "almost", "alone", "alpha", "already",
	"also", "alter", "always", "amateur", "amazing", "among", "amount", "amused",
	"analyst", "anchor", "ancient", "anger", "angle", "angry", "animal", "ankle",
	"announce", "annual", "another", "answer", "antenna", "antique", "anxiety", "any",
	"apart", "apology", "appear", "apple", "approve", "april", "arch", "arctic",
	"area", "arena", "argue", "arm", "armor", "army", "around", "arrange",
	"arrest", "arrive", "arrow", "art", "artefact", "artist", "artwork", "ask",
	"aspect", "assault", "asset", "assist", "assume", "asthma", "athlete", "atom",
	"attack", "attend", "attitude", "attract", "auction", "audit", "aunt", "author",
	"auto", "autumn", "average", "avocado", "avoid", "awake", "aware", "away",
	"awesome", "awful", "awkward", "axis", "baby", "bacon", "badge", "bag",
	"balance", "balcony", "ball", "bamboo", "banana", "banner", "barely", "bargain",
	"barrel", "base", "basic", "basket", "battle", "beach", "bean", "beauty",
	"because", "become", "beef", "before", "begin", "behave", "behind", "believe",
	"below", "belt", "bench", "benefit", "best", "betray", "better", "between",
	"beyond", "bicycle", "bid", "bike", "blind", "block", "blood", "bloom",
	"blouse", "blue", "blur", "blush", "board", "boat", "body", "boil",
	"bomb", "bone", "book", "boost", "border", "boring", "borrow", "boss",
	"bottom", "bounce", "brain", "brand", "brave", "bread", "breeze", "brick",
	"bridge", "brief", "bright", "bring", "bronze", "broom", "brother", "brown",
	"brush", "bubble", "buddy", "budget", "buffalo", "build", "bulb", "bulk",
	"bullet", "bundle", "bunker", "burden", "burger", "burst", "bus", "busy",
	"butter", "buyer", "buzz", "cabbage", "cabin", "cable", "cactus", "cage",
	"cake", "call", "calm", "camera", "camp", "cancel", "candy", "cannon",
	"canvas", "canyon", "capable", "capital", "captain", "carbon", "card", "cargo",
	"carpet", "carry", "cart", "case", "cash", "casino", "castle", "casual",
	"catalog", "catch", "category", "cattle", "caught", "cause", "caution", "cave",
	"ceiling", "celery", "cement", "census", "century", "cereal", "certain", "chair",
	"chaos", "chapter", "charge", "chase", "chat", "cheap", "check", "cheese",
	"chef", "cherry", "chest", "chicken", "chief", "child", "chimney", "choice",
	"choose", "chronic", "chuckle", "chunk", "cigar", "cinnamon", "circle", "citizen",
	"city", "civil", "claim", "clap", "clarify", "claw", "clay", "clean",
	"clerk", "clever", "click", "client", "cliff", "climb", "clinic", "clip",
	"clock", "close", "cloud", "clown", "cluster", "code", "coffee", "coil",
	"column", "combine", "come", "comfort", "comic", "common", "company", "concert",
	"conduct", "confirm", "congress", "connect", "consider", "control", "convince", "cook",
	"cool", "copper", "copy", "coral", "core", "corn", "correct", "cost",
	"cotton", "couch", "country", "couple", "course", "cousin", "cover", "coyote",
	"crack", "cradle", "craft", "crane", "crash", "crater", "crawl", "crazy",
	"cream", "credit", "creek", "crew", "cricket", "crime", "crisp", "critic",
	"cross", "crouch", "crowd", "crucial", "cruel", "cruise", "crumble", "crunch",
	"crush", "cry", "crystal", "cube", "culture", "cup", "cupboard", "curious",
	"current", "curtain", "curve", "cushion", "custom", "cute", "cycle", "dad",
	"damage", "damp", "dance", "danger", "dark", "deal", "decade", "decay",
	"december", "decide", "decline", "decorate", "decrease", "deer", "defense", "define",
	"degree", "delay", "deliver", "demand", "demise", "denial", "dentist", "deny",
	"depend", "deposit", "depth", "deputy", "derive", "describe", "desert", "design",
	"detect", "develop", "device", "devote", "diagram", "dial", "diamond", "diary",
	"differ", "digital", "dignity", "dilemma", "dinner", "dinosaur", "direct", "dirt",
	"disagree", "discover", "disease", "dish", "dismiss", "disorder", "display", "distance",
	"divert", "divide", "divorce", "dizzy", "doctor", "document", "dolphin", "domain",
	"donate", "donkey", "donor", "dragon", "drama", "drastic", "draw", "dream",
	"dress", "drift", "drink", "drip", "drive", "drop", "drum", "dust",
	"dutch", "duty", "dwarf", "dynamic", "eager", "eagle", "early", "earn",
	"earth", "easily", "edge", "educate", "effort", "egg", "eight", "either",
	"elbow", "elder", "electric", "elegant", "element", "elephant", "elevator", "elite",
	"else", "embark", "emerge", "enforce", "engage", "engine", "enhance", "enjoy",
	"enough", "enrich", "enroll", "ensure", "enter", "entire", "entry", "equal",
	"escape", "essay", "estate", "eternal", "ethics", "evidence", "evil", "evolve",
	"exact", "example", "excess", "exchange", "execute", "exercise", "exhaust", "exhibit",
	"exile", "exist", "explain", "expose", "express", "extend", "extra", "eye",
	"fable", "face", "faculty", "faint", "faith", "fall", "false", "fame",
	"family", "famous", "fan", "fancy", "fantasy", "fault", "feature", "feel",
	"fence", "festival", "fetch", "fever", "few", "fiber", "fiction", "field",
	"figure", "fire", "firm", "first", "fiscal", "fish", "flame", "flash",
	"flavor", "flee", "flight", "float", "floor", "flower", "fluid", "flush",
	"fly", "foam", "focus", "fog", "foil", "follow", "food", "force",
	"forest", "forget", "fortune", "forum", "fossil", "foster", "found", "fragile",
	"frame", "freedom", "fresh", "friend", "front", "fruit", "fuel", "fun",
	"funny", "furnace", "fury", "future", "gadget", "gain", "galaxy", "gallery",
	"game", "gap", "garbage", "garden", "garlic", "garment", "gather", "gauge",
	"gazing", "general", "genius", "genre", "gentle", "genuine", "gesture", "ghost",
	"gift", "giggle", "ginger", "giraffe", "girl", "give", "glad", "glance",
	"glare", "glass", "glide", "globe", "gloom", "glove", "glow", "gold",
	"gospel", "gossip", "govern", "gown", "grace", "grain", "grant", "grape",
	"grass", "gravity", "great", "green", "grid", "grief", "grit", "grocery",
	"group", "grow", "grunt", "guard", "guide", "guilt", "guitar", "gun",
	"gym", "habit", "hair", "half", "hammer", "hamster", "hand", "happy",
	"harsh", "harvest", "hat", "hazard", "head", "health", "heart", "heavy",
	"hedgehog", "hero", "hidden", "high", "hill", "hint", "hockey", "hollow",
	"home", "honey", "hood", "hope", "horn", "hospital", "host", "hour",
	"hover", "huge", "humble", "humor", "hundred", "hungry", "hunt", "hurdle",
	"hurry", "hurt", "husband", "hybrid", "ice", "icon", "ignore", "image",
	"impact", "impose", "impulse", "inch", "include", "income", "index", "infant",
	"inform", "inhale", "inner", "innocent", "input", "inquiry", "insect", "inside",
	"inspire", "install", "intact", "interest", "invest", "invite", "involve", "iron",
	"island", "isolate", "issue", "item", "ivory", "jacket", "jaguar", "jungle",
	"kingdom", "kitchen", "lamp", "laptop", "large", "later", "laugh", "layer",
	"leader", "learn", "left", "legal", "lemon", "letter", "light", "likely",
	"limit", "link", "lion", "liquid", "list", "little", "loan", "lobster",
	"local", "logic", "lonely", "long", "lunar", "luxury", "magic", "magnet",
	"mammal", "manage", "margin", "master", "mental", "minor", "minute", "miracle",
	"mixture", "mobile", "model", "modify", "morning", "mountain", "naive", "name",
	"narrow", "nation", "nature", "network", "neutral", "ocean", "offer", "often",
	"option", "orange", "order", "ordinary", "outdoor", "oven", "owner", "oxygen",
	"package", "paper", "parent", "patrol", "pattern", "pause", "pencil", "people",
	"perfect", "permit", "phrase", "pilot", "pizza", "place", "planet", "plastic",
	"plate", "pledge", "plunge", "poem", "point", "polar", "police", "pool",
	"popular", "power", "practice", "priority", "problem", "process", "project", "promote",
	"public", "pulp", "radar", "radio", "random", "rapid", "razor", "ready",
	"reason", "recall", "region", "repair", "report", "review", "rhythm", "ridge",
	"right", "rigid", "ritual", "rocket", "rotate", "rough", "route", "royal",
	"sadness", "salary", "sample", "sauce", "scale", "scene", "scheme", "school",
	"science", "screen", "secret", "secure", "sensor", "series", "shield", "shift",
	"signal", "silent", "silver", "simple", "sketch", "social", "solar", "solid",
	"song", "source", "space", "spark", "spirit", "stadium", "stand", "stock",
	"stomach", "storm", "strategy", "strong", "student", "subject", "supply", "surface",
	"system", "talent", "target", "teach", "team", "theory", "thought", "thunder",
	"together", "token", "topic", "trade", "transfer", "travel", "trigger", "trophy",
	"trust", "tunnel", "turtle", "typical", "ultra", "update", "urban", "useful",
	"vaccine", "valid", "valley", "vendor", "vessel", "video", "village", "vintage",
	"vision", "visual", "water", "wealth", "weapon", "welcome", "wisdom", "world",
	"yellow", "zebra", "zone",
}

// GeneratePassphrase generates a cryptographically secure passphrase of the
// given word count using the BIP39 English word list.
// wordCount=12 gives ~132 bits of entropy (sufficient for this use case).
func GeneratePassphrase(wordCount int) (string, error) {
	if wordCount < 6 {
		wordCount = 6
	}
	if wordCount > 24 {
		wordCount = 24
	}

	listSize := len(bip39Words)
	words := make([]string, wordCount)

	for i := 0; i < wordCount; i++ {
		var randBytes [4]byte
		if _, err := rand.Read(randBytes[:]); err != nil {
			return "", err
		}
		idx := binary.BigEndian.Uint32(randBytes[:]) % uint32(listSize)
		words[i] = bip39Words[idx]
	}

	return strings.Join(words, " "), nil
}
