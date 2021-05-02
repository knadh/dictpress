async function hFetch(url, method, data) {
  return new Promise((resolve, reject) => {
    fetch(url, {
      method: method || 'GET',
      body: JSON.stringify(data),
      mode: 'cors',
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
      },
    }).then((response) => {
      if (response.ok) {
        resolve(response.json());
      } else {
        reject(response);
      }
    }).catch((error) => {
      reject(error);
    });
  });
}

export const getConfig = async () => hFetch('/admin/api/config');

export const updateEntry = async (data) => hFetch('/admin/api/entries', 'put', data);

export const search = async (q, fromLang, toLang) => hFetch(`/api/dictionary/${fromLang}/${toLang}/${encodeURIComponent(q)}`, 'get');
