export interface AnimeType {
    mal_id: number;
    title: string;
    url: string;
    image: string;
    episodes: number;
    aired: {
        from: string;
        to: string;
    };
    synopsis: string;
}