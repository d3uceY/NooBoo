package services

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Packs are folders beside Game in the install. Switching one off writes the
// same packstoskipmount line the game's own Pack Selection screen writes.

var packCodeRe = regexp.MustCompile(`(?i)^(EP|GP|SP|FP)(\d{2})$`)

var packSkipLineRe = regexp.MustCompile(`(?im)^[ \t]*packstoskipmount[ \t]*=[ \t]*([^\r\n]*)`)

var packSkipReplaceRe = regexp.MustCompile(`(?im)^([ \t]*packstoskipmount[ \t]*=)[^\r\n]*`)

var userSettingSectionRe = regexp.MustCompile(`(?im)^[ \t]*\[usersetting\][ \t]*\r?\n`)

// PackService lists and toggles the game's own packs.
type PackService struct {
	cfg *ConfigService
}

func NewPackService(cfg *ConfigService) *PackService { return &PackService{cfg: cfg} }

// Root is the detected install folder, or "" when none was found.
func (s *PackService) Root() string { return detectInstall(s.cfg) }

// List reads the installed packs and the skip line. No install found is empty.
func (s *PackService) List() ([]models.Pack, error) {
	empty := []models.Pack{}
	install := detectInstall(s.cfg)
	if install == "" {
		return empty, nil
	}
	entries, err := os.ReadDir(install)
	if err != nil {
		return empty, nil
	}

	skip := readPacksToSkip(readUserSetting(s.cfg))
	packs := []models.Pack{}
	for _, e := range entries {
		if !e.IsDir() || !packCodeRe.MatchString(e.Name()) {
			continue
		}
		code := strings.ToUpper(e.Name())
		name := packNames[code]
		if name == "" {
			name = code
		}
		packs = append(packs, models.Pack{
			Code:    code,
			Name:    name,
			Kind:    packKind(code),
			Enabled: !skip[code],
		})
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Code < packs[j].Code })
	return packs, nil
}

// SetEnabled adds or removes one code from packstoskipmount.
func (s *PackService) SetEnabled(code string, enabled bool) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !packCodeRe.MatchString(code) {
		return errors.New("unknown pack code")
	}
	root := s.cfg.Get().GameRoot
	if root == "" {
		return errors.New("set your Sims 4 folder first")
	}
	path := filepath.Join(root, "UserSetting.ini")
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("the game has not written UserSetting.ini yet")
	}
	next := applyPackSkip(string(raw), code, enabled)
	if next == string(raw) {
		return nil
	}
	return os.WriteFile(path, []byte(next), 0o644)
}

func readPacksToSkip(ini string) map[string]bool {
	out := map[string]bool{}
	m := packSkipLineRe.FindStringSubmatch(ini)
	if m == nil {
		return out
	}
	for _, code := range strings.Split(m[1], ",") {
		if code = strings.ToUpper(strings.TrimSpace(code)); code != "" {
			out[code] = true
		}
	}
	return out
}

// applyPackSkip rewrites only the skip line, leaving the rest of the file alone.
func applyPackSkip(ini, code string, enabled bool) string {
	codes := readPacksToSkip(ini)
	if enabled {
		delete(codes, code)
	} else {
		codes[code] = true
	}
	list := make([]string, 0, len(codes))
	for c := range codes {
		list = append(list, c)
	}
	sort.Strings(list)
	value := strings.Join(list, ",")

	if packSkipReplaceRe.MatchString(ini) {
		return packSkipReplaceRe.ReplaceAllString(ini, "${1} "+value)
	}
	if value == "" {
		return ini
	}
	nl := "\n"
	if strings.Contains(ini, "\r\n") {
		nl = "\r\n"
	}
	if loc := userSettingSectionRe.FindStringIndex(ini); loc != nil {
		return ini[:loc[1]] + "packstoskipmount = " + value + nl + nl + ini[loc[1]:]
	}
	tail := ""
	if ini != "" && !strings.HasSuffix(ini, "\n") {
		tail = nl
	}
	return ini + tail + "[usersetting]" + nl + "packstoskipmount = " + value + nl
}

func readUserSetting(cfg *ConfigService) string {
	root := cfg.Get().GameRoot
	if root == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(root, "UserSetting.ini"))
	if err != nil {
		return ""
	}
	return string(b)
}

func packKind(code string) string {
	m := packCodeRe.FindStringSubmatch(code)
	if m == nil {
		return "pack"
	}
	switch strings.ToUpper(m[1]) {
	case "EP":
		return "expansion"
	case "GP":
		return "game pack"
	case "FP":
		return "free"
	}
	n, _ := strconv.Atoi(m[2])
	if n <= 18 || n == 46 || n == 49 {
		return "stuff pack"
	}
	return "kit"
}

// looksLikeInstall tells a game install apart from the user folder.
func looksLikeInstall(dir string) bool {
	for _, sub := range []string{"Game", "Data"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func detectInstall(cfg *ConfigService) string {
	if root := cfg.Get().InstallRoot; root != "" && looksLikeInstall(root) {
		return root
	}
	for _, p := range installCandidates() {
		if looksLikeInstall(p) {
			return p
		}
	}
	return ""
}

func installCandidates() []string {
	var out []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if p := os.Getenv(env); p != "" {
			out = append(out,
				filepath.Join(p, "EA Games", "The Sims 4"),
				filepath.Join(p, "Electronic Arts", "The Sims 4"),
			)
		}
	}
	for d := 'A'; d <= 'Z'; d++ {
		root := string(d) + ":\\"
		out = append(out,
			filepath.Join(root, "EA Games", "The Sims 4"),
			filepath.Join(root, "Program Files", "EA Games", "The Sims 4"),
			filepath.Join(root, "SteamLibrary", "steamapps", "common", "The Sims 4"),
			filepath.Join(root, "Steam", "steamapps", "common", "The Sims 4"),
		)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		for _, lib := range steamLibraries(home) {
			out = append(out, filepath.Join(lib, "steamapps", "common", "The Sims 4"))
		}
	}
	return out
}

// PickInstallFolder asks for the install and remembers it when it is real.
func (c *ConfigService) PickInstallFolder() (string, error) {
	dir, err := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Select The Sims 4 install folder").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}
	if !looksLikeInstall(dir) {
		return "", errors.New("that folder is not a Sims 4 install")
	}
	cfg := c.Get()
	cfg.InstallRoot = dir
	return dir, c.save(cfg)
}

// packNames are EA's own titles. A code missing here draws as itself.
var packNames = map[string]string{
	"EP01": "Get to Work",
	"EP02": "Get Together",
	"EP03": "City Living",
	"EP04": "Cats & Dogs",
	"EP05": "Seasons",
	"EP06": "Get Famous",
	"EP07": "Island Living",
	"EP08": "Discover University",
	"EP09": "Eco Lifestyle",
	"EP10": "Snowy Escape",
	"EP11": "Cottage Living",
	"EP12": "High School Years",
	"EP13": "Growing Together",
	"EP14": "Horse Ranch",
	"EP15": "For Rent",
	"EP16": "Lovestruck",
	"EP17": "Life & Death",
	"EP18": "Businesses & Hobbies",
	"EP19": "Enchanted by Nature",
	"EP20": "Adventure Awaits",
	"EP21": "Royalty & Legacy",
	"FP01": "Holiday Celebration Pack",
	"GP01": "Outdoor Retreat",
	"GP02": "Spa Day",
	"GP03": "Dine Out",
	"GP04": "Vampires",
	"GP05": "Parenthood",
	"GP06": "Jungle Adventure",
	"GP07": "StrangerVille",
	"GP08": "Realm of Magic",
	"GP09": "Star Wars: Journey to Batuu",
	"GP10": "Dream Home Decorator",
	"GP11": "My Wedding Stories",
	"GP12": "Werewolves",
	"SP01": "Luxury Party Stuff",
	"SP02": "Perfect Patio Stuff",
	"SP03": "Cool Kitchen Stuff",
	"SP04": "Spooky Stuff",
	"SP05": "Movie Hangout Stuff",
	"SP06": "Romantic Garden Stuff",
	"SP07": "Kids Room Stuff",
	"SP08": "Backyard Stuff",
	"SP09": "Vintage Glamour Stuff",
	"SP10": "Bowling Night Stuff",
	"SP11": "Fitness Stuff",
	"SP12": "Toddler Stuff",
	"SP13": "Laundry Day Stuff",
	"SP14": "My First Pet Stuff",
	"SP15": "Moschino Stuff",
	"SP16": "Tiny Living",
	"SP17": "Nifty Knitting",
	"SP18": "Paranormal Stuff",
	"SP20": "Throwback Fit Kit",
	"SP21": "Country Kitchen Kit",
	"SP22": "Bust the Dust Kit",
	"SP23": "Courtyard Oasis Kit",
	"SP24": "Fashion Street Kit",
	"SP25": "Industrial Loft Kit",
	"SP26": "Incheon Arrivals Kit",
	"SP28": "Modern Menswear Kit",
	"SP29": "Blooming Rooms Kit",
	"SP30": "Carnaval Streetwear Kit",
	"SP31": "Décor to the Max Kit",
	"SP32": "Moonlight Chic Kit",
	"SP33": "Little Campers Kit",
	"SP34": "First Fits Kit",
	"SP35": "Desert Luxe Kit",
	"SP36": "Pastel Pop Kit",
	"SP37": "Everyday Clutter Kit",
	"SP38": "Simtimates Collection Kit",
	"SP39": "Bathroom Clutter Kit",
	"SP40": "Greenhouse Haven Kit",
	"SP41": "Basement Treasures Kit",
	"SP42": "Grunge Revival Kit",
	"SP43": "Book Nook Kit",
	"SP44": "Poolside Splash Kit",
	"SP45": "Modern Luxe Kit",
	"SP46": "Home Chef Hustle",
	"SP47": "Castle Estate Kit",
	"SP48": "Goth Galore Kit",
	"SP49": "Crystal Creations",
	"SP50": "Urban Homage Kit",
	"SP51": "Party Essentials Kit",
	"SP52": "Riviera Retreat Kit",
	"SP53": "Cozy Bistro Kit",
	"SP54": "Artist Studio Kit",
	"SP55": "Storybook Nursery Kit",
	"SP56": "Sweet Slumber Party Kit",
	"SP57": "Cozy Kitsch Kit",
	"SP58": "Comfy Gamer Kit",
	"SP59": "Secret Sanctuary Kit",
	"SP60": "Casanova Cave Kit",
	"SP61": "Refined Livingroom Kit",
	"SP62": "Business Chic Kit",
	"SP63": "Sleek Bathroom Kit",
	"SP64": "Sweet Allure Kit",
	"SP65": "Restoration Workshop Kit",
	"SP66": "Golden Years Kit",
	"SP67": "Kitchen Clutter Kit",
	"SP68": "SpongeBob's House Kit",
	"SP69": "Autumn Apparel Kit",
	"SP70": "SpongeBob Kid's Room Kit",
	"SP71": "Grange Mudroom Kit",
	"SP72": "Essential Glam Kit",
	"SP73": "Modern Retreat Kit",
	"SP74": "Garden to Table Kit",
	"SP75": "Wonderland Playroom Kit",
	"SP76": "Silver Screen Style Kit",
	"SP77": "Tea Time Solarium Kit",
	"SP78": "Lady Bridgerton's Masquerade Ball Fashion Kit",
	"SP79": "Lady Bridgerton's Masquerade Ballroom Kit",
	"SP81": "Prairie Dreams Kit",
	"SP82": "Yard Charm Kit",
}
