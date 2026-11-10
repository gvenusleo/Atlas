package gui

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// defaultScheme 是偏好里没有配色、或配色不认识时用的配色的 id。
const defaultScheme = "github"

// schemeChoice 是 schemeChoices 里的一项：id 存进偏好，name 显示在设置页里。
type schemeChoice struct {
	id, name string
	scheme   scheme
}

// schemeChoices 是设置页里可选的配色，按名称的字母顺序排列。
var schemeChoices = []schemeChoice{
	{"adwaita", "Adwaita", scheme{
		light: palette{
			canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F6F5F4"),
			border: ui.Hex("#DCDCDC"),
			text:   ui.Hex("#1D1D20"), muted: ui.Hex("#5D5D5D"),
			accent: ui.Hex("#12488B"), accentFg: ui.Hex("#12488B"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#C01C28"), success: ui.Hex("#1A7A4A"), running: ui.Hex("#8A5F35"),
		},
		dark: palette{
			canvas: ui.Hex("#1C1C1F"), subtle: ui.Hex("#26262A"),
			border: ui.Hex("#3D3D43"),
			text:   ui.Hex("#F6F5F4"), muted: ui.Hex("#9C9AA2"),
			accent: ui.Hex("#2F6FC7"), accentFg: ui.Hex("#7AB4FF"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#FF6B6B"), success: ui.Hex("#57E389"), running: ui.Hex("#F8E45C"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#1D1D20"), ui.Hex("#C01C28"), ui.Hex("#26A269"), ui.Hex("#A2734C"),
			ui.Hex("#12488B"), ui.Hex("#A347BA"), ui.Hex("#2AA1B3"), ui.Hex("#C2C2C2"),
			ui.Hex("#5D5D5D"), ui.Hex("#F66151"), ui.Hex("#33D17A"), ui.Hex("#E9AD0C"),
			ui.Hex("#2A7BDE"), ui.Hex("#C061CB"), ui.Hex("#33C7DE"), ui.Hex("#FFFFFF"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#241F31"), ui.Hex("#C01C28"), ui.Hex("#2EC27E"), ui.Hex("#F5C211"),
			ui.Hex("#1E78E4"), ui.Hex("#9841BB"), ui.Hex("#0AB9DC"), ui.Hex("#C0BFBC"),
			ui.Hex("#5E5C64"), ui.Hex("#ED333B"), ui.Hex("#57E389"), ui.Hex("#F8E45C"),
			ui.Hex("#51A1FF"), ui.Hex("#C061CB"), ui.Hex("#4FD2FD"), ui.Hex("#F6F5F4"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#C01C28"), ui.Hex("#1D7B50"), ui.Hex("#12488B"),
			ui.Hex("#A045B6"), ui.Hex("#8D6442"), ui.Hex("#1F7785"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#F15C62"), ui.Hex("#57E389"), ui.Hex("#51A1FF"),
			ui.Hex("#C56DCF"), ui.Hex("#F8E45C"), ui.Hex("#4FD2FD"),
		},
	}},
	{"apple", "Apple", scheme{
		light: palette{
			canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F5F5F7"),
			border: ui.Hex("#D9D9DE"),
			text:   ui.Hex("#000000"), muted: ui.Hex("#464646"),
			accent: ui.Hex("#0869CB"), accentFg: ui.Hex("#0869CB"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#CC372E"), success: ui.Hex("#1B7A2A"), running: ui.Hex("#8A6D00"),
		},
		dark: palette{
			canvas: ui.Hex("#1E1E1E"), subtle: ui.Hex("#282828"),
			border: ui.Hex("#3A3A3C"),
			text:   ui.Hex("#FFFFFF"), muted: ui.Hex("#A8A8AD"),
			accent: ui.Hex("#1F6BC4"), accentFg: ui.Hex("#5AA9FF"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#FF6B62"), success: ui.Hex("#32D74B"), running: ui.Hex("#FFD60A"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#1A1A1A"), ui.Hex("#CC372E"), ui.Hex("#26A439"), ui.Hex("#CDAC08"),
			ui.Hex("#0869CB"), ui.Hex("#9647BF"), ui.Hex("#479EC2"), ui.Hex("#98989D"),
			ui.Hex("#464646"), ui.Hex("#FF453A"), ui.Hex("#32D74B"), ui.Hex("#E5BC00"),
			ui.Hex("#0A84FF"), ui.Hex("#BF5AF2"), ui.Hex("#69C9F2"), ui.Hex("#FFFFFF"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#1A1A1A"), ui.Hex("#CC372E"), ui.Hex("#26A439"), ui.Hex("#CDAC08"),
			ui.Hex("#0869CB"), ui.Hex("#9647BF"), ui.Hex("#479EC2"), ui.Hex("#98989D"),
			ui.Hex("#464646"), ui.Hex("#FF453A"), ui.Hex("#32D74B"), ui.Hex("#FFD60A"),
			ui.Hex("#0A84FF"), ui.Hex("#BF5AF2"), ui.Hex("#76D6FF"), ui.Hex("#FFFFFF"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#C8362D"), ui.Hex("#1D7D2C"), ui.Hex("#0869CB"),
			ui.Hex("#9647BF"), ui.Hex("#806B05"), ui.Hex("#34748E"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#FF5248"), ui.Hex("#32D74B"), ui.Hex("#2391FF"),
			ui.Hex("#C467F3"), ui.Hex("#FFD60A"), ui.Hex("#76D6FF"),
		},
	}},
	{"claude", "Claude", scheme{
		light: palette{
			canvas: ui.Hex("#FAF9F5"), subtle: ui.Hex("#F0EEE6"),
			border: ui.Hex("#E0DCD0"),
			text:   ui.Hex("#141413"), muted: ui.Hex("#51504D"),
			accent: ui.Hex("#B85A36"), accentFg: ui.Hex("#A84B3A"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#B91C1C"), success: ui.Hex("#2E7C4C"), running: ui.Hex("#8A6220"),
		},
		dark: palette{
			canvas: ui.Hex("#262624"), subtle: ui.Hex("#2F2E2B"),
			border: ui.Hex("#3D3D3C"),
			text:   ui.Hex("#E5E4E1"), muted: ui.Hex("#A6A59B"),
			accent: ui.Hex("#D97757"), accentFg: ui.Hex("#E08A6D"),
			onAccent: ui.Hex("#262624"),
			danger:   ui.Hex("#F87171"), success: ui.Hex("#9ACA86"), running: ui.Hex("#E8C96B"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#0A0A0A"), ui.Hex("#A84B3A"), ui.Hex("#2E7C4C"), ui.Hex("#8A6220"),
			ui.Hex("#184E95"), ui.Hex("#882B4D"), ui.Hex("#066049"), ui.Hex("#B6B5AA"),
			ui.Hex("#51504D"), ui.Hex("#E34A4A"), ui.Hex("#639900"), ui.Hex("#B87700"),
			ui.Hex("#3886E5"), ui.Hex("#D55382"), ui.Hex("#199F70"), ui.Hex("#E2E1DA"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#888681"), ui.Hex("#D47563"), ui.Hex("#9ACA86"), ui.Hex("#E8C96B"),
			ui.Hex("#6A9BCC"), ui.Hex("#9B87F5"), ui.Hex("#3CBE8C"), ui.Hex("#E2E1DA"),
			ui.Hex("#A6A59B"), ui.Hex("#F4A9A9"), ui.Hex("#A8D166"), ui.Hex("#FAB319"),
			ui.Hex("#9FC5F4"), ui.Hex("#F3AAC5"), ui.Hex("#79D7B3"), ui.Hex("#EFEEEB"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#A84B3A"), ui.Hex("#2C7649"), ui.Hex("#184E95"),
			ui.Hex("#882B4D"), ui.Hex("#896220"), ui.Hex("#066049"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#F4A9A9"), ui.Hex("#A8D166"), ui.Hex("#9FC5F4"),
			ui.Hex("#F3AAC5"), ui.Hex("#E8C96B"), ui.Hex("#79D7B3"),
		},
	}},
	{"cursor", "Cursor", scheme{
		light: palette{
			canvas: ui.Hex("#FBFBFB"), subtle: ui.Hex("#F3F3F3"),
			border: ui.Hex("#D6D6D6"),
			text:   ui.Hex("#252525"), muted: ui.Hex("#5C5C5C"),
			accent: ui.Hex("#2F6A94"), accentFg: ui.Hex("#2F6A94"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#B8284B"), success: ui.Hex("#17714F"), running: ui.Hex("#8A5A00"),
		},
		dark: palette{
			canvas: ui.Hex("#141414"), subtle: ui.Hex("#1E1E1E"),
			border: ui.Hex("#303030"),
			text:   ui.Hex("#D4D4D4"), muted: ui.Hex("#9A9A9A"),
			accent: ui.Hex("#87A6C4"), accentFg: ui.Hex("#A5BED9"),
			onAccent: ui.Hex("#141414"),
			danger:   ui.Hex("#FC6B83"), success: ui.Hex("#3FA266"), running: ui.Hex("#D2943E"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#252525"), ui.Hex("#CF2D56"), ui.Hex("#1F8A65"), ui.Hex("#A16900"),
			ui.Hex("#3C7CAB"), ui.Hex("#B8448B"), ui.Hex("#4C7F8C"), ui.Hex("#AFAFAF"),
			ui.Hex("#5C5C5C"), ui.Hex("#E75E78"), ui.Hex("#55A583"), ui.Hex("#C08532"),
			ui.Hex("#6299C3"), ui.Hex("#D06BA6"), ui.Hex("#6F9BA6"), ui.Hex("#FFFFFF"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#242424"), ui.Hex("#FC6B83"), ui.Hex("#3FA266"), ui.Hex("#D2943E"),
			ui.Hex("#81A1C1"), ui.Hex("#B48EAD"), ui.Hex("#88C0D0"), ui.Hex("#E4E4E4"),
			ui.Hex("#4A4A4A"), ui.Hex("#FC6B83"), ui.Hex("#70B489"), ui.Hex("#F1B467"),
			ui.Hex("#87A6C4"), ui.Hex("#B48EAD"), ui.Hex("#88C0D0"), ui.Hex("#E4E4E4"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#BE294F"), ui.Hex("#1A7355"), ui.Hex("#346A93"),
			ui.Hex("#A73E7F"), ui.Hex("#8C5B00"), ui.Hex("#416C77"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#FC6B83"), ui.Hex("#70B489"), ui.Hex("#87A6C4"),
			ui.Hex("#B48EAD"), ui.Hex("#F1B467"), ui.Hex("#88C0D0"),
		},
	}},
	{"flexoki", "Flexoki", scheme{
		light: palette{
			canvas: ui.Hex("#FFFCF0"), subtle: ui.Hex("#F2F0E5"),
			border: ui.Hex("#DAD5C4"),
			text:   ui.Hex("#100F0F"), muted: ui.Hex("#6F6E69"),
			accent: ui.Hex("#205EA6"), accentFg: ui.Hex("#205EA6"),
			onAccent: ui.Hex("#FFFCF0"),
			danger:   ui.Hex("#AF3029"), success: ui.Hex("#5A7009"), running: ui.Hex("#8A6900"),
		},
		dark: palette{
			canvas: ui.Hex("#100F0F"), subtle: ui.Hex("#1C1B1A"),
			border: ui.Hex("#343331"),
			text:   ui.Hex("#CECDC3"), muted: ui.Hex("#A8A69F"),
			accent: ui.Hex("#4385BE"), accentFg: ui.Hex("#7FB0D9"),
			onAccent: ui.Hex("#100F0F"),
			danger:   ui.Hex("#E06457"), success: ui.Hex("#879A39"), running: ui.Hex("#D0A215"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#100F0F"), ui.Hex("#AF3029"), ui.Hex("#66800B"), ui.Hex("#AD8301"),
			ui.Hex("#205EA6"), ui.Hex("#A02F6F"), ui.Hex("#24837B"), ui.Hex("#6F6E69"),
			ui.Hex("#B7B5AC"), ui.Hex("#D14D41"), ui.Hex("#879A39"), ui.Hex("#D0A215"),
			ui.Hex("#4385BE"), ui.Hex("#CE5D97"), ui.Hex("#3AA99F"), ui.Hex("#CECDC3"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#100F0F"), ui.Hex("#D14D41"), ui.Hex("#879A39"), ui.Hex("#D0A215"),
			ui.Hex("#4385BE"), ui.Hex("#CE5D97"), ui.Hex("#3AA99F"), ui.Hex("#878580"),
			ui.Hex("#575653"), ui.Hex("#AF3029"), ui.Hex("#66800B"), ui.Hex("#AD8301"),
			ui.Hex("#205EA6"), ui.Hex("#A02F6F"), ui.Hex("#24837B"), ui.Hex("#CECDC3"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#AF3029"), ui.Hex("#5C730A"), ui.Hex("#205EA6"),
			ui.Hex("#A02F6F"), ui.Hex("#866601"), ui.Hex("#21766F"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#D66157"), ui.Hex("#879A39"), ui.Hex("#4B8BC1"),
			ui.Hex("#CE5F98"), ui.Hex("#D0A215"), ui.Hex("#3AA99F"),
		},
	}},
	{"github", "GitHub", scheme{
		light: palette{
			canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F6F8FA"),
			border: ui.Hex("#D1D9E0"),
			text:   ui.Hex("#1F2328"), muted: ui.Hex("#59636E"),
			accent: ui.Hex("#0969DA"), accentFg: ui.Hex("#0969DA"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#D1242F"), success: ui.Hex("#1A7F37"), running: ui.Hex("#9A6700"),
		},
		dark: palette{
			canvas: ui.Hex("#0D1117"), subtle: ui.Hex("#151B23"),
			border: ui.Hex("#3D444D"),
			text:   ui.Hex("#F0F6FC"), muted: ui.Hex("#9198A1"),
			accent: ui.Hex("#1F6FEB"), accentFg: ui.Hex("#4493F8"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#F85149"), success: ui.Hex("#3FB950"), running: ui.Hex("#D29922"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#24292F"), ui.Hex("#CF222E"), ui.Hex("#116329"), ui.Hex("#4D2D00"),
			ui.Hex("#0969DA"), ui.Hex("#8250DF"), ui.Hex("#1B7C83"), ui.Hex("#6E7781"),
			ui.Hex("#57606A"), ui.Hex("#A40E26"), ui.Hex("#1A7F37"), ui.Hex("#633C01"),
			ui.Hex("#218BFF"), ui.Hex("#A475F9"), ui.Hex("#3192AA"), ui.Hex("#8C959F"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#484F58"), ui.Hex("#FF7B72"), ui.Hex("#3FB950"), ui.Hex("#D29922"),
			ui.Hex("#58A6FF"), ui.Hex("#BC8CFF"), ui.Hex("#39C5CF"), ui.Hex("#B1BAC4"),
			ui.Hex("#6E7681"), ui.Hex("#FFA198"), ui.Hex("#56D364"), ui.Hex("#E3B341"),
			ui.Hex("#79C0FF"), ui.Hex("#D2A8FF"), ui.Hex("#56D4DD"), ui.Hex("#FFFFFF"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#CF222E"), ui.Hex("#0A3069"), ui.Hex("#0550AE"),
			ui.Hex("#8250DF"), ui.Hex("#953800"), ui.Hex("#116329"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#FF7B72"), ui.Hex("#A5D6FF"), ui.Hex("#79C0FF"),
			ui.Hex("#D2A8FF"), ui.Hex("#FFA657"), ui.Hex("#7EE787"),
		},
	}},
	{"raycast", "Raycast", scheme{
		light: palette{
			canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F7F7F7"),
			border: ui.Hex("#E5E5E5"),
			text:   ui.Hex("#000000"), muted: ui.Hex("#6E6E6E"),
			accent: ui.Hex("#0968C2"), accentFg: ui.Hex("#0968C2"),
			onAccent: ui.Hex("#FFFFFF"),
			danger:   ui.Hex("#B12424"), success: ui.Hex("#006B4F"), running: ui.Hex("#96600A"),
		},
		dark: palette{
			canvas: ui.Hex("#1A1A1A"), subtle: ui.Hex("#232323"),
			border: ui.Hex("#333333"),
			text:   ui.Hex("#FFFFFF"), muted: ui.Hex("#9A9A9A"),
			accent: ui.Hex("#56C2FF"), accentFg: ui.Hex("#7DD1FF"),
			onAccent: ui.Hex("#14141A"),
			danger:   ui.Hex("#FF5360"), success: ui.Hex("#59D499"), running: ui.Hex("#FFC531"),
		},
		lightANSI: [16]ui.Color{
			ui.Hex("#000000"), ui.Hex("#B12424"), ui.Hex("#006B4F"), ui.Hex("#F8A300"),
			ui.Hex("#138AF2"), ui.Hex("#9A1B6E"), ui.Hex("#3EB8BF"), ui.Hex("#BFBFBF"),
			ui.Hex("#000000"), ui.Hex("#B12424"), ui.Hex("#006B4F"), ui.Hex("#F8A300"),
			ui.Hex("#138AF2"), ui.Hex("#9A1B6E"), ui.Hex("#3EB8BF"), ui.Hex("#FFFFFF"),
		},
		darkANSI: [16]ui.Color{
			ui.Hex("#000000"), ui.Hex("#FF5360"), ui.Hex("#59D499"), ui.Hex("#FFC531"),
			ui.Hex("#56C2FF"), ui.Hex("#CF2F98"), ui.Hex("#52EEE5"), ui.Hex("#FFFFFF"),
			ui.Hex("#4C4C4C"), ui.Hex("#FF6363"), ui.Hex("#59D499"), ui.Hex("#FFC531"),
			ui.Hex("#56C2FF"), ui.Hex("#CF2F98"), ui.Hex("#52EEE5"), ui.Hex("#FFFFFF"),
		},
		lightTokens: [6]ui.Color{
			ui.Hex("#B12424"), ui.Hex("#006B4F"), ui.Hex("#0F6EC2"),
			ui.Hex("#9A1B6E"), ui.Hex("#956200"), ui.Hex("#28777C"),
		},
		darkTokens: [6]ui.Color{
			ui.Hex("#FF6363"), ui.Hex("#59D499"), ui.Hex("#56C2FF"),
			ui.Hex("#DA60B0"), ui.Hex("#FFC531"), ui.Hex("#52EEE5"),
		},
	}},
}

// schemeIndex 是 id 在 schemeChoices 里的位置；不认识的 id 取默认配色的位置。
func schemeIndex(id string) int {
	find := func(id string) int {
		return slices.IndexFunc(schemeChoices, func(it schemeChoice) bool { return it.id == id })
	}
	if i := find(id); i >= 0 {
		return i
	}
	return find(defaultScheme)
}
