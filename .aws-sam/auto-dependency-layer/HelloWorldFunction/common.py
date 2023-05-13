import concurrent


def multithreaded_batch(MAX_PROC, entities_to_load):
    with concurrent.futures.ThreadPoolExecutor(max_workers=MAX_PROC) as executor:
        for entity_to_load in entities_to_load:
            data_list = entities_to_load[entity_to_load]
            batches = chunks(data_list, 25)
            for batch in batches:
                futures.append(executor.submit(dal.dynamo.load_batch, entity_to_load, batch))
        for future in concurrent.futures.as_completed(futures):
            results.append(future.result())
            
def chunks(lst, n):
    """Yield successive n-sized chunks from lst."""
    for i in range(0, len(lst), n):
        yield lst[i:i + n] 