// khan
// https://github.com/topfreegames/khan
//
// Licensed under the MIT license:
// http://www.opensource.org/licenses/mit-license
// Copyright © 2016 Top Free Games <backend@tfgco.com>

package api_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/satori/go.uuid"
	. "github.com/topfreegames/khan/api"
	"github.com/topfreegames/khan/models"
	kt "github.com/topfreegames/khan/testing"
)

func startRouteHandler(routes []string, port int) *[]map[string]interface{} {
	responses := []map[string]interface{}{}

	go func() {
		handleFunc := func(w http.ResponseWriter, r *http.Request) {
			bs, err := ioutil.ReadAll(r.Body)
			if err != nil {
				responses = append(responses, map[string]interface{}{"reason": err})
				return
			}

			var payload map[string]interface{}
			json.Unmarshal(bs, &payload)

			response := map[string]interface{}{
				"payload":  payload,
				"request":  r,
				"response": w,
			}

			responses = append(responses, response)
		}
		for _, route := range routes {
			http.HandleFunc(route, handleFunc)
		}

		http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), nil)
	}()

	return &responses
}

var _ = Describe("API Application", func() {
	var testDb models.DB

	BeforeEach(func() {
		var err error
		testDb, err = GetTestDB()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("App Struct", func() {
		It("should create app with custom arguments", func() {
			l := kt.NewMockLogger()
			app := GetApp("127.0.0.1", 9999, "../config/test.yaml", false, l, false)
			Expect(app.Port).To(Equal(9999))
			Expect(app.Host).To(Equal("127.0.0.1"))
		})
	})

	Describe("App Games", func() {
		It("should load all games", func() {
			game := models.GameFactory.MustCreate().(*models.Game)
			err := testDb.Insert(game)
			Expect(err).NotTo(HaveOccurred())

			app := GetDefaultTestApp()

			appGame, err := app.GetGame(game.PublicID)
			Expect(err).NotTo(HaveOccurred())
			Expect(appGame.ID).To(Equal(game.ID))
		})

		It("should get game by Public ID", func() {
			game := models.GameFactory.MustCreate().(*models.Game)
			err := testDb.Insert(game)
			Expect(err).NotTo(HaveOccurred())

			app := GetDefaultTestApp()

			appGame, err := app.GetGame(game.PublicID)
			Expect(err).NotTo(HaveOccurred())

			Expect(appGame.ID).To(Equal(game.ID))
		})
	})

	Describe("App Load Hooks", func() {
		It("should load all hooks", func() {
			gameID := uuid.NewV4().String()
			_, err := models.GetTestHooks(testDb, gameID, 2)
			Expect(err).NotTo(HaveOccurred())

			app := GetDefaultTestApp()

			hooks := app.GetHooks()
			Expect(len(hooks[gameID])).To(Equal(2))
			Expect(len(hooks[gameID][0])).To(Equal(2))
			Expect(len(hooks[gameID][1])).To(Equal(2))
		})
	})

	Describe("App Dispatch Hook", func() {
		It("should dispatch hooks", func() {
			hooks, err := models.GetHooksForRoutes(testDb, []string{
				"http://localhost:52525/created",
				"http://localhost:52525/created2",
			}, models.GameUpdatedHook)
			Expect(err).NotTo(HaveOccurred())
			responses := startRouteHandler([]string{"/created", "/created2"}, 52525)

			app := GetDefaultTestApp()
			time.Sleep(time.Second)

			resultingPayload := map[string]interface{}{
				"success":  true,
				"publicID": hooks[0].GameID,
			}
			err = app.DispatchHooks(hooks[0].GameID, models.GameUpdatedHook, resultingPayload)
			Expect(err).NotTo(HaveOccurred())
			app.Dispatcher.Wait()
			Expect(len(*responses)).To(Equal(2))
			app.Errors.Tick()
			Expect(app.Errors.Rate()).To(Equal(0.0))
		})

		It("should encode hook parameters", func() {
			hooks, err := models.GetHooksForRoutes(
				testDb, []string{
					"http://localhost:52525/encoding?url={{url}}",
				}, models.GameUpdatedHook,
			)
			Expect(err).NotTo(HaveOccurred())
			responses := startRouteHandler(
				[]string{"/encoding"},
				52525,
			)

			app := GetDefaultTestApp()
			time.Sleep(time.Second)

			resultingPayload := map[string]interface{}{
				"url":      "http://some-url.com",
				"success":  true,
				"publicID": hooks[0].GameID,
			}
			err = app.DispatchHooks(
				hooks[0].GameID,
				models.GameUpdatedHook,
				resultingPayload,
			)
			Expect(err).NotTo(HaveOccurred())
			app.Dispatcher.Wait()
			Expect(len(*responses)).To(Equal(1))

			resp := (*responses)[0]
			req := resp["request"].(*http.Request)

			url := req.URL.Query().Get("url")
			Expect(url).To(Equal("http://some-url.com"))

			app.Errors.Tick()
			Expect(app.Errors.Rate()).To(Equal(0.0))
		})

		It("should dispatch hooks using template", func() {
			hooks, err := models.GetHooksForRoutes(testDb, []string{
				"http://localhost:52525/created/{{publicID}}",
			}, models.GameUpdatedHook)
			Expect(err).NotTo(HaveOccurred())
			responses := startRouteHandler([]string{fmt.Sprintf("/created/%s", hooks[0].GameID)}, 52525)

			app := GetDefaultTestApp()
			time.Sleep(time.Second)

			resultingPayload := map[string]interface{}{
				"success":  true,
				"publicID": hooks[0].GameID,
			}
			err = app.DispatchHooks(hooks[0].GameID, models.GameUpdatedHook, resultingPayload)
			Expect(err).NotTo(HaveOccurred())
			app.Dispatcher.Wait()
			Expect(len(*responses)).To(Equal(1))
			app.Errors.Tick()
			Expect(app.Errors.Rate()).To(Equal(0.0))
		})

		It("should dispatch hooks using second-level key", func() {
			hooks, err := models.GetHooksForRoutes(testDb, []string{
				"http://localhost:52525/{{playerPosition}}/créated/{{player.publicID}}",
			}, models.GameUpdatedHook)
			Expect(err).NotTo(HaveOccurred())
			responses := startRouteHandler([]string{fmt.Sprintf("/1/créated/%s", hooks[0].GameID)}, 52525)

			app := GetDefaultTestApp()
			time.Sleep(time.Second)

			resultingPayload := map[string]interface{}{
				"success":        true,
				"playerPosition": 1,
				"player": map[string]interface{}{
					"publicID": hooks[0].GameID,
				},
			}
			err = app.DispatchHooks(hooks[0].GameID, models.GameUpdatedHook, resultingPayload)
			Expect(err).NotTo(HaveOccurred())
			app.Dispatcher.Wait()
			Expect(len(*responses)).To(Equal(1))
			app.Errors.Tick()
			Expect(app.Errors.Rate()).To(Equal(0.0))
		})

		It("should fail dispatch hooks if invalid key", func() {
			hooks, err := models.GetHooksForRoutes(testDb, []string{
				"http://localhost:52525/invalid/{{player.publicID.invalid}}",
			}, models.GameUpdatedHook)
			Expect(err).NotTo(HaveOccurred())
			responses := startRouteHandler([]string{fmt.Sprintf("/invalid/%s", hooks[0].GameID)}, 52525)

			app := GetDefaultTestApp()
			time.Sleep(time.Second)

			resultingPayload := map[string]interface{}{
				"success": true,
				"player": map[string]interface{}{
					"publicID": hooks[0].GameID,
				},
			}
			err = app.DispatchHooks(hooks[0].GameID, models.GameUpdatedHook, resultingPayload)
			Expect(err).NotTo(HaveOccurred())
			app.Dispatcher.Wait(50)
			Expect(len(*responses)).To(Equal(0))
			app.Errors.Tick()
			Expect(app.Errors.Rate()).To(BeNumerically(">", 0.0))
		})
	})
})
