// This file is Free Software under the MIT License
// without warranty, see README.md and LICENSES/MIT.txt for details.
//
// SPDX-License-Identifier: Apache-2.0
//
// SPDX-FileCopyrightText: 2024 German Federal Office for Information Security (BSI) <https://www.bsi.bund.de>
//  Software-Engineering: 2024 Intevation GmbH <https://intevation.de>

import { writable } from "svelte/store";

type AppStore = {
  app: {
    userProfile: {
      firstName: string;
      lastName: string;
    };
    isUserLoggedIn: boolean;
    token: any;
    keycloak: any;
  };
};

const generateInitalState = (): AppStore => {
  const state = {
    app: {
      userProfile: {
        firstName: "",
        lastName: ""
      },
      isUserLoggedIn: false,
      token: null,
      keycloak: null
    }
  };
  return state;
};

function createStore() {
  const { subscribe, set, update } = writable(generateInitalState());
  return {
    subscribe,
    setKeycloak: (keycloak: any) => {
      update((settings) => {
        settings.app.keycloak = keycloak;
        return settings;
      });
    },
    setUserProfile: (userProfile: any) => {
      update((settings) => {
        const { firstName, lastName } = userProfile;
        settings.app.userProfile.firstName = firstName;
        settings.app.userProfile.lastName = lastName;
        return settings;
      });
    },
    setLoginState: (newState: boolean) => {
      update((settings) => {
        settings.app.isUserLoggedIn = newState;
        return settings;
      });
    },
    reset: () => {
      set(generateInitalState());
    }
  };
}

export const appStore = createStore();
