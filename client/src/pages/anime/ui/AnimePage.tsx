import {AnimeType} from "../model/types";

export default async function Anime() {
  const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/anime`);
  const response = await res.json();

  return (
    <div>
      <h1>Anime Catalog</h1>
      <ul>
        {response.data?.map((anime: AnimeType) => (
          <li key={anime.mal_id}>{anime.title}</li>
        ))}
      </ul>
    </div>
  );
}