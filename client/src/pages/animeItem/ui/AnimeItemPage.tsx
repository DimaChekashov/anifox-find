export default async function AnimeItemPage({params}: {params: {id: string}}) {
  const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/anime/${params.id}`);
  const data = await res.json();

  return (
    <div>
      <h1>{data.title}</h1>
    </div>
  );
}