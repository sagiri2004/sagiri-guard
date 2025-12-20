package firewall

// CategoryDomains định nghĩa danh sách domain theo category
var CategoryDomains = map[string][]string{
	"social_media": {
		"facebook.com",
		"www.facebook.com",
		"twitter.com",
		"www.twitter.com",
		"x.com",
		"www.x.com",
		"instagram.com",
		"www.instagram.com",
		"tiktok.com",
		"www.tiktok.com",
		"linkedin.com",
		"www.linkedin.com",
		"youtube.com",
		"www.youtube.com",
		"reddit.com",
		"www.reddit.com",
		"pinterest.com",
		"www.pinterest.com",
		"snapchat.com",
		"www.snapchat.com",
		"discord.com",
		"www.discord.com",
		"telegram.org",
		"web.telegram.org",
	},
	"ai": {
		"chatgpt.com",
		"www.chatgpt.com",
		"openai.com",
		"www.openai.com",
		"claude.ai",
		"www.claude.ai",
		"bard.google.com",
		"gemini.google.com",
		"perplexity.ai",
		"www.perplexity.ai",
		"character.ai",
		"www.character.ai",
		"midjourney.com",
		"www.midjourney.com",
		"stability.ai",
		"www.stability.ai",
	},
	"gaming": {
		"steamcommunity.com",
		"store.steampowered.com",
		"epicgames.com",
		"www.epicgames.com",
		"roblox.com",
		"www.roblox.com",
		"twitch.tv",
		"www.twitch.tv",
		"battlenet.com",
		"www.battlenet.com",
		"origin.com",
		"www.origin.com",
	},
	"shopping": {
		"amazon.com",
		"www.amazon.com",
		"ebay.com",
		"www.ebay.com",
		"shopee.vn",
		"www.shopee.vn",
		"lazada.vn",
		"www.lazada.vn",
		"tiki.vn",
		"www.tiki.vn",
		"aliexpress.com",
		"www.aliexpress.com",
	},
	"news": {
		"vnexpress.net",
		"www.vnexpress.net",
		"dantri.com.vn",
		"www.dantri.com.vn",
		"tuoitre.vn",
		"www.tuoitre.vn",
		"thanhnien.vn",
		"www.thanhnien.vn",
		"bbc.com",
		"www.bbc.com",
		"cnn.com",
		"www.cnn.com",
	},
	"entertainment": {
		"netflix.com",
		"www.netflix.com",
		"disney.com",
		"www.disney.com",
		"hulu.com",
		"www.hulu.com",
		"spotify.com",
		"www.spotify.com",
		"soundcloud.com",
		"www.soundcloud.com",
	},
	"adult": {
		"pornhub.com",
		"www.pornhub.com",
		"xvideos.com",
		"www.xvideos.com",
		"xhamster.com",
		"www.xhamster.com",
	},
}

// GetDomainsByCategory lấy danh sách domain theo category
func GetDomainsByCategory(category string) []string {
	return CategoryDomains[category]
}

// GetAllCategories lấy tất cả category names
func GetAllCategories() []string {
	cats := make([]string, 0, len(CategoryDomains))
	for cat := range CategoryDomains {
		cats = append(cats, cat)
	}
	return cats
}



