package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
)

type Pokemon struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Height  int    `json:"height"`
	Weight  int    `json:"weight"`
	Sprites struct {
		FrontDefault string `json:"front_default"`
	} `json:"sprites"`
	Cries struct {
		Latest string `json:"latest"`
	} `json:"cries"`
	Types []struct {
		Type struct {
			Name string `json:"name"`
		} `json:"type"`
	} `json:"types"`
	Abilities []struct {
		Ability struct {
			Name string `json:"name"`
		} `json:"ability"`
	} `json:"abilities"`
	Stats []struct {
		BaseStat int `json:"base_stat"`
		Stat     struct {
			Name string `json:"name"`
		} `json:"stat"`
	} `json:"stats"`
	Moves []struct {
		Move struct {
			Name string `json:"name"`
		} `json:"move"`
	} `json:"moves"`
}

const MAX_POKEMON_ID = 1025

var (
	currentPokemon *Pokemon
	audioMutex     sync.Mutex
	isPlaying      bool
)

func getPokemonData(id int) (*Pokemon, error) {
	resp, err := http.Get(fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%d", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var pokemon Pokemon
	err = json.Unmarshal(body, &pokemon)
	if err != nil {
		return nil, err
	}

	return &pokemon, nil
}

func updatePokemonDisplay(pokemonName *widget.Label, pokemonImage *canvas.Image, pokemonStats *widget.Label, id int) {
	if id < 1 {
		id = MAX_POKEMON_ID
	}
	pokemon, err := getPokemonData(id)
	if err != nil {
		pokemonName.SetText("Error: Pokémon not found")
		currentPokemon = nil
		return
	}

	currentPokemon = pokemon
	pokemonName.SetText(fmt.Sprintf("#%d %s", pokemon.ID, pokemon.Name))
	pokemonStats.SetText(fmt.Sprintf("Height: %d\nWeight: %d", pokemon.Height, pokemon.Weight))

	// Load and display the Pokémon image
	res, _ := http.Get(pokemon.Sprites.FrontDefault)
	if res != nil {
		defer res.Body.Close()
		img, _, _ := image.Decode(res.Body)
		if img != nil {
			pokemonImage.Image = img
			pokemonImage.Refresh()
		}
	}
}

var infoTypes = []string{"Basic", "Stats", "Abilities", "Moves"}
var currentInfoIndex = 0

func main() {
	a := app.New()
	w := a.NewWindow("Pokedex")

	redColor := color.NRGBA{R: 220, G: 30, B: 30, A: 255}
	blueColor := color.NRGBA{R: 100, G: 200, B: 255, A: 255}

	// ui elements
	outerFrame := canvas.NewRectangle(redColor)
	outerFrame.SetMinSize(fyne.NewSize(300, 500))
	screenFrame := canvas.NewRectangle(blueColor)
	screenFrame.SetMinSize(fyne.NewSize(260, 300))
	pokemonName := widget.NewLabel("Pokémon Name")
	pokemonImage := canvas.NewImageFromResource(nil)
	pokemonImage.FillMode = canvas.ImageFillContain
	pokemonImage.SetMinSize(fyne.NewSize(200, 200))
	pokemonStats := widget.NewLabel("Height: \nWeight: ")

	// Initial Pokémon display
	updatePokemonDisplay(pokemonName, pokemonImage, pokemonStats, 1)

	// Search elements
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter Pokémon name...")
	searchButton := widget.NewButton("Search", func() {
		id, err := strconv.Atoi(searchEntry.Text)
		if err != nil {
			resp, err := http.Get(fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%s", searchEntry.Text))
			if err != nil {
				pokemonName.SetText("Error: Pokémon not found")
				return
			}
			defer resp.Body.Close()
			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)
			id = int(result["id"].(float64))
		}
		updatePokemonDisplay(pokemonName, pokemonImage, pokemonStats, id)
	})

	// Button layout
	buttonLayout := container.NewGridWithColumns(5,
		widget.NewButton("", nil),
		widget.NewButton("", nil),
		widget.NewButton("", nil),
		widget.NewButton("", nil),
		widget.NewButton("", nil),
	)
	for _, btn := range buttonLayout.Objects {
		btn.(*widget.Button).Importance = widget.LowImportance
	}

	// D-pad layout with functionality
	dPad := container.NewGridWithColumns(3,
		layout.NewSpacer(),
		widget.NewButton("▲", func() {
			if currentPokemon != nil {
				updatePokemonDisplay(pokemonName, pokemonImage, pokemonStats, currentPokemon.ID-1)
			}
		}),
		layout.NewSpacer(),
		widget.NewButton("◄", func() {
			currentInfoIndex = (currentInfoIndex - 1 + len(infoTypes)) % len(infoTypes)
			updatePokemonInfo(pokemonStats)
		}),
		widget.NewButton("●", func() {
			if currentPokemon != nil {
				go func() {
					if err := playPokemonCry(currentPokemon.Cries.Latest); err != nil {
						fmt.Println("Error playing Pokémon cry:", err)
					}
				}()
			}
		}),
		widget.NewButton("►", func() {
			currentInfoIndex = (currentInfoIndex + 1) % len(infoTypes)
			updatePokemonInfo(pokemonStats)
		}),
		layout.NewSpacer(),
		widget.NewButton("▼", func() {
			if currentPokemon != nil {
				updatePokemonDisplay(pokemonName, pokemonImage, pokemonStats, currentPokemon.ID+1)
			}
		}),
		layout.NewSpacer(),
	)

	content := container.NewVBox(
		container.NewCenter(pokemonName),
		container.NewCenter(pokemonImage),
		container.NewCenter(pokemonStats),
		searchEntry,
		searchButton,
	)
	screenContent := container.NewBorder(nil, buttonLayout, nil, nil, content)
	pokedexLayout := container.NewBorder(
		container.NewHBox(layout.NewSpacer()),
		dPad,
		nil, nil,
		container.NewCenter(
			container.NewStack(
				screenFrame,
				screenContent,
			),
		),
	)
	w.SetContent(container.NewStack(
		outerFrame,
		container.NewPadded(pokedexLayout),
	))

	w.Resize(fyne.NewSize(320, 520))
	w.ShowAndRun()
}
func updatePokemonInfo(pokemonStats *widget.Label) {
	if currentPokemon == nil {
		pokemonStats.SetText("No Pokémon selected")
		return
	}

	var info string
	switch infoTypes[currentInfoIndex] {
	case "Basic":
		info = fmt.Sprintf("Height: %d\nWeight: %d", currentPokemon.Height, currentPokemon.Weight)
	case "Stats":
		info = "Stats:\n"
		for _, stat := range currentPokemon.Stats {
			info += fmt.Sprintf("%s: %d\n", stat.Stat.Name, stat.BaseStat)
		}
	case "Abilities":
		info = "Abilities:\n"
		for _, ability := range currentPokemon.Abilities {
			info += fmt.Sprintf("- %s\n", ability.Ability.Name)
		}
	case "Moves":
		info = "Moves:\n"
		for i, move := range currentPokemon.Moves {
			if i >= 5 {
				break
			}
			info += fmt.Sprintf("- %s\n", move.Move.Name)
		}
		info += "(showing first 5 moves)"
	}

	pokemonStats.SetText(info)
}

func playPokemonCry(url string) error {
	audioMutex.Lock()
	if isPlaying {
		audioMutex.Unlock()
		return nil
	}
	isPlaying = true
	audioMutex.Unlock()

	defer func() {
		audioMutex.Lock()
		isPlaying = false
		audioMutex.Unlock()
	}()

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get audio: %v", err)
	}
	defer resp.Body.Close()

	streamer, format, err := vorbis.Decode(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decode audio: %v", err)
	}
	defer streamer.Close()

	err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	if err != nil {
		speaker.Unlock()
		return fmt.Errorf("failed to initialize speaker: %v", err)
	}

	done := make(chan bool)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	<-done

	return nil
}
